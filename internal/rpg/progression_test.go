package rpg

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

func TestProgressionEconomyAndEquipment(t *testing.T) {
	db, s, p := fixture(t)
	ctx := context.Background()
	p.Coins = 300
	p.TrainingXP = 1000
	patchProfile(t, db, p)
	a := ProgressRequest{RequestID: "training-first", Revision: p.Revision, CharacterID: "char_001", Levels: 1}
	out, err := s.Progress(ctx, p.ID, "train", a)
	if err != nil {
		t.Fatal(err)
	}
	if out.Profile.Coins != 290 || out.Profile.TrainingXP != 900 || out.Profile.Growth["char_001"].Level != 2 {
		t.Fatal("training debit/level mismatch")
	}
	retry, err := s.Progress(ctx, p.ID, "train", a)
	if err != nil || !reflect.DeepEqual(out, retry) {
		t.Fatal("training replay changed state", err)
	}
	a.RequestID = "training-forged"
	a.Revision = out.Profile.Revision
	a.CharacterID = "char_060"
	_, err = s.Progress(ctx, p.ID, "train", a)
	requireCode(t, err, "character_unowned")
	a.CharacterID = "char_001"
	a.Levels = -1
	a.RequestID = "training-negative"
	_, err = s.Progress(ctx, p.ID, "train", a)
	requireCode(t, err, "levels")
	a.Levels = 10
	a.RequestID = "training-cap"
	_, err = s.Progress(ctx, p.ID, "train", a)
	requireCode(t, err, "level_cap")
	buy := ProgressRequest{RequestID: "buy-first-blade", Revision: out.Profile.Revision, ItemID: "blade_dawn"}
	out, err = s.Progress(ctx, p.ID, "buy", buy)
	if err != nil {
		t.Fatal(err)
	}
	retry, err = s.Progress(ctx, p.ID, "buy", buy)
	if err != nil || retry.Profile.Inventory["blade_dawn"] != 1 || retry.Profile.Coins != 260 {
		t.Fatal("purchase replay", err)
	}
	equip := ProgressRequest{RequestID: "equip-first-blade", Revision: out.Profile.Revision, CharacterID: "char_001", ItemID: "blade_dawn", Slot: "weapon"}
	out, err = s.Progress(ctx, p.ID, "equip", equip)
	if err != nil {
		t.Fatal(err)
	}
	equip.RequestID = "equip-other-blade"
	equip.Revision = out.Profile.Revision
	equip.CharacterID = "char_002"
	_, err = s.Progress(ctx, p.ID, "equip", equip)
	requireCode(t, err, "equipment_unavailable")
	equip.RequestID = "equip-wrong-slot"
	equip.Slot = "armor"
	_, err = s.Progress(ctx, p.ID, "equip", equip)
	requireCode(t, err, "equipment")
	buy.RequestID = "buy-locked-blade"
	buy.Revision = out.Profile.Revision
	buy.ItemID = "blade_aurora"
	_, err = s.Progress(ctx, p.ID, "buy", buy)
	requireCode(t, err, "equipment_locked")
	started, err := s.StartBattle(ctx, p.ID, "progression-start", 0)
	if err != nil {
		t.Fatal(err)
	}
	h := started.Battle.Heroes[0]
	if h.Level != 2 || h.Attack != rounded(42*1.0105)+8 {
		t.Fatal("equipment/level stats not used", h.Level, h.Attack)
	}
	a.RequestID = "training-in-battle"
	a.Revision = started.Profile.Revision
	a.Levels = 1
	_, err = s.Progress(ctx, p.ID, "train", a)
	requireCode(t, err, "battle_active")
	winBattle(t, s, p.ID, started.Battle, "progression-win")
	out, err = s.Snapshot(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if out.Profile.Growth["char_001"].Level != 3 || out.Profile.TrainingXP != 1000 || out.Battle.RewardXP != 100 {
		t.Fatal("first clear XP", out.Profile.Growth, out.Profile.TrainingXP)
	}
	replay, err := s.StartBattle(ctx, p.ID, "progression-replay", 0)
	if err != nil {
		t.Fatal(err)
	}
	winBattle(t, s, p.ID, replay.Battle, "progression-replay-win")
	after, err := s.Snapshot(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after.Profile.Growth, out.Profile.Growth) || after.Profile.TrainingXP != out.Profile.TrainingXP || after.Profile.Coins != out.Profile.Coins {
		t.Fatal("replay farmed rewards")
	}
	restarted, err := New(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := restarted.Snapshot(ctx, p.ID)
	if err != nil || !reflect.DeepEqual(saved.Profile, after.Profile) {
		t.Fatal("progress lost after service restart", err)
	}
}

func TestLegacyProfileAndBattleRemainPlayable(t *testing.T) {
	db, s, p := fixture(t)
	ctx := context.Background()
	p.Unlocked = 98
	p.Version = 1
	p.Growth = nil
	p.Inventory = nil
	p.Loadouts = nil
	patchProfile(t, db, p)
	out, err := s.Snapshot(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if out.Profile.Growth["char_001"].Level != 99 || out.Profile.Shards != p.Shards || out.Profile.Coins != p.Coins {
		t.Fatal("migration reset progress")
	}
	out, err = s.StartBattle(ctx, p.ID, "legacy-battle", 98)
	if err != nil {
		t.Fatal(err)
	}
	b := out.Battle
	b.Rules = "arunika-v1"
	for i := range b.Heroes {
		b.Heroes[i].Level = 0
	}
	raw, _ := json.Marshal(b)
	if _, err = db.Exec(`UPDATE rpg_battles SET state=? WHERE id=?`, string(raw), b.ID); err != nil {
		t.Fatal(err)
	}
	next, err := s.Act(ctx, p.ID, b.ID, Action{RequestID: "legacy-skill", Revision: b.Revision, Actor: 0, Target: 0, Action: "skill"})
	if err != nil {
		t.Fatal(err)
	}
	if next.Battle.Enemies[0].Burn != 0 || next.Battle.Rules != "arunika-v1" {
		t.Fatal("old battle used new mechanics")
	}
	winBattle(t, s, p.ID, next.Battle, "legacy-finish")
	out, err = s.Snapshot(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if out.Profile.Growth["char_001"].Level != 100 {
		t.Fatal("legacy completion lost XP")
	}
}

func TestAllCharacterKitsAreUsable(t *testing.T) {
	_, s, _ := fixture(t)
	for _, c := range s.catalog.Characters {
		t.Run(c.ID, func(t *testing.T) {
			b := &Battle{Rules: RulesVersion, Stage: 0, Round: 1, Heroes: []Hero{{Character: c, Level: 1, HP: 100, Max: 300, Attack: 50, Defense: 20, Energy: 3}, {Character: s.chars["char_001"], Level: 1, HP: 50, Max: 250, Energy: 1}}, Enemies: []Opponent{{HP: 500, Max: 600, Attack: 30, Defense: 20, Shield: 40, Charged: true, Mark: true, Burn: 2, BurnPower: 12}, {HP: 500, Max: 600, Attack: 30, Defense: 20}}}
			if err := s.useSkill(b, 0, 0); err != nil {
				t.Fatal(err)
			}
			if b.Heroes[0].Energy > 2 || b.Heroes[0].Energy < 1 {
				t.Fatal("energy outside expected range", b.Heroes[0].Energy)
			}
			if c.Role == "Medic" || c.Role == "Guardian" {
				if b.Heroes[0].HP == 100 && b.Heroes[1].HP == 50 && b.Heroes[0].Shield == 0 && b.Heroes[1].Shield == 0 && !b.Heroes[0].Guard && !b.Heroes[1].Guard {
					t.Fatal("support skill had no effect")
				}
			} else if b.Enemies[0].HP == 500 && b.Enemies[0].Shield == 40 {
				t.Fatal("offensive skill had no effect")
			}
		})
	}
}

func TestBurnRetargetAndShieldPersistence(t *testing.T) {
	db, s, p := fixture(t)
	ctx := context.Background()
	out, err := s.StartBattle(ctx, p.ID, "burn-battle", 0)
	if err != nil {
		t.Fatal(err)
	}
	b := out.Battle
	b.Enemies[0].HP = 1
	b.Enemies[0].Burn = 2
	b.Enemies[0].BurnPower = 10
	for i := range b.Heroes {
		b.Heroes[i].Acted = true
	}
	b.Heroes[0].Acted = false
	b.Heroes[0].Shield = 50
	raw, _ := json.Marshal(b)
	if _, err = db.Exec(`UPDATE rpg_battles SET state=? WHERE id=?`, string(raw), b.ID); err != nil {
		t.Fatal(err)
	}
	out, err = s.Act(ctx, p.ID, b.ID, Action{RequestID: "burn-finish-phase", Revision: b.Revision, Actor: 0, Target: 0, Action: "guard"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Battle.Target != 1 || out.Battle.Round != 2 {
		t.Fatal("burn left dead selected target")
	}
	saved, err := s.Snapshot(ctx, p.ID)
	if err != nil || !reflect.DeepEqual(saved.Battle, out.Battle) {
		t.Fatal("statuses lost", err)
	}
}

func TestTrainAndBuyConcurrentRevision(t *testing.T) {
	db, s, p := fixture(t)
	p.Coins = 30
	p.TrainingXP = 100
	patchProfile(t, db, p)
	results := make(chan error, 2)
	for i := range 2 {
		go func(i int) {
			_, err := s.Progress(context.Background(), p.ID, "buy", ProgressRequest{RequestID: fmt.Sprintf("buy-concurrent-%d", i), Revision: p.Revision, ItemID: "blade_dawn"})
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
	out, err := s.Snapshot(context.Background(), p.ID)
	if err != nil || success != 1 || out.Profile.Coins != 0 || out.Profile.Inventory["blade_dawn"] != 1 {
		t.Fatal("double spending", err)
	}
}
