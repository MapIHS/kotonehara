package rpg

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

func TestAwakeningAndEnhancementPersistAndApply(t *testing.T) {
	db, s, p := fixture(t)
	ctx := context.Background()
	p.Coins = 3000
	p.Dust = 1000
	p.Inventory["blade_dawn"] = 2
	p.Loadouts["char_001"] = map[string]string{"weapon": "blade_dawn"}
	p.Loadouts["char_002"] = map[string]string{"weapon": "blade_dawn"}
	patchProfile(t, db, p)
	a := ProgressRequest{RequestID: "awaken-first", Revision: p.Revision, CharacterID: "char_001"}
	out, err := s.Progress(ctx, p.ID, "awaken", a)
	if err != nil {
		t.Fatal(err)
	}
	if out.Profile.Awakening["char_001"] != 1 || out.Profile.Coins != 2900 || out.Profile.Dust != 980 || !reflect.DeepEqual(out.Profile.Collection, p.Collection) {
		t.Fatal("awakening economy", out.Profile)
	}
	retry, err := s.Progress(ctx, p.ID, "awaken", a)
	if err != nil || !reflect.DeepEqual(out, retry) {
		t.Fatal("awakening replay", err)
	}
	a = ProgressRequest{RequestID: "enhance-first", Revision: out.Profile.Revision, ItemID: "blade_dawn"}
	out, err = s.Progress(ctx, p.ID, "enhance", a)
	if err != nil {
		t.Fatal(err)
	}
	if out.Profile.Enhancements["blade_dawn"] != 1 || out.Profile.Coins != 2870 || out.Profile.Dust != 975 || out.Profile.Inventory["blade_dawn"] != 2 {
		t.Fatal("enhancement economy")
	}
	retry, err = s.Progress(ctx, p.ID, "enhance", a)
	if err != nil || !reflect.DeepEqual(out, retry) {
		t.Fatal("enhancement replay", err)
	}
	restarted, err := New(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := restarted.Snapshot(ctx, p.ID)
	if err != nil || !reflect.DeepEqual(saved.Profile, out.Profile) {
		t.Fatal("restart lost upgrades", err)
	}
	started, err := restarted.StartBattle(ctx, p.ID, "upgraded-battle", 0)
	if err != nil {
		t.Fatal(err)
	}
	h := started.Battle.Heroes[0]
	if h.Awakening != 1 || h.Max != 231 || h.Attack != 53 || h.Defense != 27 {
		t.Fatal("awakening + enhanced equipment stats", h)
	}
	// Two copies use the same learned enhancement; zero equipment stats stay zero.
	if started.Battle.Heroes[1].Attack != rounded(29*1.04)+9 || started.Battle.Heroes[1].Max != rounded(290*1.04) {
		t.Fatal("shared enhancement/zero HP")
	}
	for _, op := range []string{"awaken", "enhance"} {
		_, err = s.Progress(ctx, p.ID, op, ProgressRequest{RequestID: "locked-" + op, Revision: started.Profile.Revision, CharacterID: "char_001", ItemID: "blade_dawn"})
		requireCode(t, err, "battle_active")
	}
}

func TestAdvancementBoundsAndFundsAreAtomic(t *testing.T) {
	for _, tc := range []struct {
		name, op, char, item, code   string
		coins, dust, awaken, enhance int
	}{
		{"no character", "awaken", "char_060", "", "character_unowned", 9999, 9999, 0, 0},
		{"no item", "enhance", "", "coat_moss", "equipment_unowned", 9999, 9999, 0, 0},
		{"unknown item", "enhance", "", "forged", "equipment", 9999, 9999, 0, 0},
		{"no awakening coins", "awaken", "char_001", "", "funds", 99, 9999, 0, 0},
		{"no awakening dust", "awaken", "char_001", "", "funds", 9999, 19, 0, 0},
		{"no enhancement coins", "enhance", "", "blade_dawn", "funds", 29, 9999, 0, 0},
		{"no enhancement dust", "enhance", "", "blade_dawn", "funds", 9999, 4, 0, 0},
		{"awakening cap", "awaken", "char_001", "", "awakening_max", 9999, 9999, 5, 0},
		{"enhancement cap", "enhance", "", "blade_dawn", "enhancement_max", 9999, 9999, 0, 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, s, p := fixture(t)
			p.Coins = tc.coins
			p.Dust = tc.dust
			p.Awakening["char_001"] = tc.awaken
			p.Enhancements["blade_dawn"] = tc.enhance
			p.Inventory["blade_dawn"] = 1
			patchProfile(t, db, p)
			_, err := s.Progress(context.Background(), p.ID, tc.op, ProgressRequest{RequestID: "invalid-advancement", Revision: p.Revision, CharacterID: tc.char, ItemID: tc.item})
			requireCode(t, err, tc.code)
			after, err := s.Snapshot(context.Background(), p.ID)
			if err != nil || !reflect.DeepEqual(after.Profile, p) {
				t.Fatal("failed advancement changed profile", err)
			}
		})
	}
}

func TestAdvancementConcurrentCostsAndMaximum(t *testing.T) {
	db, s, p := fixture(t)
	ctx := context.Background()
	p.Coins = 10000
	p.Dust = 10000
	patchProfile(t, db, p)
	results := make(chan error, 2)
	for i := range 2 {
		go func(i int) {
			_, err := s.Progress(ctx, p.ID, "awaken", ProgressRequest{RequestID: fmt.Sprintf("race-awaken-%d", i), Revision: p.Revision, CharacterID: "char_001"})
			results <- err
		}(i)
	}
	success := 0
	for range 2 {
		if err := <-results; err == nil {
			success++
		} else {
			requireCode(t, err, "stale_revision")
		}
	}
	if success != 1 {
		t.Fatal("double spend")
	}
	for rank := 2; rank <= 5; rank++ {
		before, err := s.Snapshot(ctx, p.ID)
		if err != nil {
			t.Fatal(err)
		}
		out, err := s.Progress(ctx, p.ID, "awaken", ProgressRequest{RequestID: fmt.Sprintf("awakening-rank-%d", rank), Revision: before.Profile.Revision, CharacterID: "char_001"})
		if err != nil {
			t.Fatal(err)
		}
		if out.Profile.Coins != before.Profile.Coins-100*rank || out.Profile.Dust != before.Profile.Dust-20*rank {
			t.Fatal("rank pricing")
		}
	}
	after, err := s.Snapshot(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Profile.Coins != 8500 || after.Profile.Dust != 9700 || after.Profile.Awakening["char_001"] != 5 {
		t.Fatal("total awakening costs")
	}
}

func TestV2BattleRetainsSkillsWithoutPassives(t *testing.T) {
	db, s, p := fixture(t)
	ctx := context.Background()
	out, err := s.StartBattle(ctx, p.ID, "legacy-v2-start", 0)
	if err != nil {
		t.Fatal(err)
	}
	b := out.Battle
	b.Rules = "arunika-v2"
	// A healed Ranu bonus must never affect a pre-v3 battle.
	b.Heroes[0].PassiveBoost = .15
	b.Enemies[0].HP = 1000
	b.Enemies[0].Max = 1000
	raw, _ := json.Marshal(b)
	if _, err = db.Exec(`UPDATE rpg_battles SET state=? WHERE id=?`, string(raw), b.ID); err != nil {
		t.Fatal(err)
	}
	out, err = s.Act(ctx, p.ID, b.ID, Action{RequestID: "legacy-v2-skill", Revision: b.Revision, Actor: 0, Action: "skill", Target: 0})
	if err != nil {
		t.Fatal(err)
	}
	if out.Battle.Enemies[0].Burn != 2 || out.Battle.Heroes[0].PassiveBoost != .15 || out.Battle.Rules != "arunika-v2" {
		t.Fatal("v2 behavior changed")
	}
	winBattle(t, s, p.ID, out.Battle, "legacy-v2-finish")
}
