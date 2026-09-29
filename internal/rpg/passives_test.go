package rpg

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

func passiveBattle(id string) *Battle {
	return &Battle{Rules: RulesVersion, Round: 1, Heroes: []Hero{{Character: Character{ID: id}, HP: 100, Max: 200, Energy: 1, Attack: 50, Defense: 20, Level: 1}, {Character: Character{ID: "char_001"}, HP: 40, Max: 200, Energy: 1}}, Enemies: []Opponent{{HP: 500, Max: 500, Defense: 20, Shield: 50, Mark: true, Blind: 1, Burn: 2, Weaken: 1, Slow: 1, Expose: 1, Charged: true}, {HP: 500, Max: 500}}}
}
func TestPassiveDamageConditionsAndConsumption(t *testing.T) {
	for _, tc := range []struct {
		id     string
		factor float64
	}{
		{"char_003", 1.15}, {"char_013", 1.2}, {"char_021", 1.15}, {"char_027", 1.15}, {"char_036", 1.15}, {"char_039", 1.15}, {"char_051", 1.1},
	} {
		t.Run(tc.id, func(t *testing.T) {
			b := passiveBattle(tc.id)
			h, e := &b.Heroes[0], &b.Enemies[0]
			if got := passivePower(b, h, e); got != tc.factor {
				t.Fatal(got, tc.factor)
			}
			e.Mark = false
			e.Blind = 0
			e.Burn = 0
			e.Charged = false
			e.Shield = 0
			e.Weaken = 0
			e.Slow = 0
			e.Expose = 0
			if got := passivePower(b, h, e); got != 1 {
				t.Fatal("bonus without condition", got)
			}
		})
	}
	b := passiveBattle("char_001")
	h := &b.Heroes[0]
	healHero(b, nil, h, 10, false)
	if passivePower(b, h, &b.Enemies[0]) != 1.15 || passivePower(b, h, &b.Enemies[0]) != 1 {
		t.Fatal("healing bonus not consumed once")
	}
	h.HP = h.Max
	healHero(b, nil, h, 10, false)
	if h.PassiveBoost != 0 {
		t.Fatal("overheal granted Ranu bonus")
	}
	b = passiveBattle("char_040")
	h = &b.Heroes[0]
	passiveAfterAction(b, h, "attack", 1)
	h.CurrentAction = "skill"
	if passivePower(b, h, &b.Enemies[0]) != 1.15 {
		t.Fatal("skill charge missing")
	}
	passiveAfterAction(b, h, "skill", 2)
	if h.SkillBoost {
		t.Fatal("skill charge not consumed")
	}
}
func TestPassiveHealingShieldAndSurvival(t *testing.T) {
	b := passiveBattle("char_023")
	source, target := &b.Heroes[0], &b.Heroes[1]
	target.HP = 190
	healHero(b, source, target, 100, true)
	if target.HP != 200 || target.Shield != 50 || target.PassiveBoost != .15 {
		t.Fatal("overheal conversion", target)
	}
	healHero(b, source, target, 100, true)
	if target.Shield != 50 {
		t.Fatal("overheal stacked shield")
	}
	b = passiveBattle("char_059")
	source, target = &b.Heroes[0], &b.Heroes[1]
	healHero(b, source, target, 20, true)
	if target.Shield != 40 || !source.PassiveUsed {
		t.Fatal("critical safeguard")
	}
	target.Shield = 0
	target.HP = 20
	healHero(b, source, target, 20, true)
	if target.Shield != 0 {
		t.Fatal("once per battle repeated")
	}
	for _, tc := range []struct {
		id   string
		want int
	}{{"char_002", 24}, {"char_032", 25}, {"char_044", 23}} {
		b = passiveBattle(tc.id)
		b.Heroes[0].HP = 90
		giveShield(b, &b.Heroes[0], &b.Heroes[1], .1)
		if b.Heroes[1].Shield != tc.want {
			t.Fatal(tc.id, b.Heroes[1].Shield)
		}
	}
	b = passiveBattle("char_038")
	source, target = &b.Heroes[0], &b.Heroes[1]
	giveShield(b, source, target, .1)
	target.Shield = 0
	passiveShieldBroken(b, target)
	if target.HP != 56 {
		t.Fatal("shield heal missing")
	}
	passiveShieldBroken(b, target)
	if target.HP != 56 {
		t.Fatal("shield heal repeated same round")
	}
	target.HP = 0
	b.Round++
	passiveShieldBroken(b, target)
	if target.HP != 0 {
		t.Fatal("shield resurrected fallen hero")
	}
	b = passiveBattle("char_050")
	source, target = &b.Heroes[0], &b.Heroes[1]
	giveShield(b, source, target, .1)
	target.Shield = 0
	passiveShieldBroken(b, target)
	passiveShieldBroken(b, target)
	if target.Energy != 2 {
		t.Fatal("shield energy repeated")
	}
	b = passiveBattle("char_025")
	giveShield(b, &b.Heroes[1], &b.Heroes[0], .1)
	giveShield(b, &b.Heroes[1], &b.Heroes[0], .2)
	if b.Heroes[0].Energy != 2 {
		t.Fatal("shield reception energy not bounded")
	}
}
func TestPassiveIncomingAndEnergyLimits(t *testing.T) {
	for _, tc := range []struct {
		id   string
		want int
	}{{"char_004", 80}, {"char_008", 65}, {"char_014", 85}, {"char_018", 0}, {"char_026", 80}, {"char_043", 90}, {"char_049", 85}} {
		b := passiveBattle(tc.id)
		h := &b.Heroes[0]
		h.HP = 90
		h.Shield = 20
		h.Guard = true
		if n := passiveIncoming(b, h, &b.Enemies[0], 100); n != tc.want {
			t.Fatal(tc.id, n)
		}
		if tc.id == "char_008" || tc.id == "char_018" {
			if passiveIncoming(b, h, &b.Enemies[0], 100) != 100 {
				t.Fatal("once per battle repeated", tc.id)
			}
		}
	}
	for _, id := range []string{"char_009", "char_045"} {
		b := passiveBattle(id)
		h := &b.Heroes[0]
		passivePower(b, h, &b.Enemies[0])
		passivePower(b, h, &b.Enemies[0])
		if h.Energy != 2 {
			t.Fatal(id, "multi hit granted duplicate energy")
		}
		b.Round++
		passivePower(b, h, &b.Enemies[0])
		if h.Energy != 3 {
			t.Fatal(id, "next round grant")
		}
	}
	for _, id := range []string{"char_012", "char_022", "char_028", "char_042", "char_053"} {
		b := passiveBattle(id)
		h := &b.Heroes[0]
		passiveAfterAction(b, h, "skill", 2)
		passiveAfterAction(b, h, "skill", 2)
		if h.Energy != 2 {
			t.Fatal(id, "duplicate skill energy")
		}
		b.Round++
		h.Energy = 5
		passiveAfterAction(b, h, "skill", 2)
		if h.Energy != 5 {
			t.Fatal("energy cap")
		}
	}
}
func TestPassiveSkillMetadataAndV2Isolation(t *testing.T) {
	_, s, _ := fixture(t)
	for _, tc := range []struct {
		id    string
		check func(*Battle) bool
	}{
		{"char_016", func(b *Battle) bool { return b.Enemies[0].BlindChance == 50 }},
		{"char_030", func(b *Battle) bool { return b.Enemies[0].Weaken == 2 }},
		{"char_046", func(b *Battle) bool {
			n := 0
			for _, e := range b.Enemies {
				if e.MarkBonus == 40 {
					n++
				}
			}
			return n == 1
		}},
		{"char_048", func(b *Battle) bool { return b.Enemies[0].BurnSource == "char_048" }},
		{"char_052", func(b *Battle) bool { return b.Enemies[1].BurnExpose }},
		{"char_057", func(b *Battle) bool { return b.Enemies[0].Burn == 4 }},
	} {
		t.Run(tc.id, func(t *testing.T) {
			b := passiveBattle(tc.id)
			b.Heroes[0].Character = s.chars[tc.id]
			b.Heroes[0].Energy = 3
			if err := s.useSkill(b, 0, 0); err != nil {
				t.Fatal(err)
			}
			if !tc.check(b) {
				t.Fatal("missing passive metadata", b.Enemies)
			}
		})
	}
	b := passiveBattle("char_023")
	b.Rules = "arunika-v2"
	b.Heroes[1].HP = 190
	healHero(b, &b.Heroes[0], &b.Heroes[1], 100, true)
	if b.Heroes[1].Shield != 0 || b.Heroes[1].PassiveBoost != 0 {
		t.Fatal("legacy overheal passive activated")
	}
	b.Heroes[0].Character = s.chars["char_016"]
	b.Heroes[0].Energy = 3
	if err := s.useSkill(b, 0, 0); err != nil {
		t.Fatal(err)
	}
	if b.Enemies[0].BlindChance != 0 {
		t.Fatal("legacy skill passive activated")
	}
}
func TestPassiveSnapshotReloadAndActionReplay(t *testing.T) {
	db, s, p := fixture(t)
	ctx := context.Background()
	out, err := s.StartBattle(ctx, p.ID, "passive-state-start", 0)
	if err != nil {
		t.Fatal(err)
	}
	b := out.Battle
	b.Heroes[0].PassiveBoost = .15
	b.Enemies[0].HP = 999
	b.Enemies[0].Max = 999
	raw, _ := json.Marshal(b)
	if _, err = db.Exec(`UPDATE rpg_battles SET state=? WHERE id=?`, string(raw), b.ID); err != nil {
		t.Fatal(err)
	}
	a := Action{RequestID: "passive-hit", Revision: b.Revision, Actor: 0, Target: 0, Action: "attack"}
	out, err = s.Act(ctx, p.ID, b.ID, a)
	if err != nil {
		t.Fatal(err)
	}
	if out.Battle.Heroes[0].PassiveBoost != 0 {
		t.Fatal("passive not consumed")
	}
	s, err = New(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.Act(ctx, p.ID, b.ID, a)
	if err != nil || !reflect.DeepEqual(out, again) {
		t.Fatal("passive replay after restart changed state", err)
	}
}

func TestBlindMissActivatesTricksterPassive(t *testing.T) {
	db, s, p := fixture(t)
	ctx := context.Background()
	out, err := s.StartBattle(ctx, p.ID, "blind-passive-start", 0)
	if err != nil {
		t.Fatal(err)
	}
	b := out.Battle
	b.Heroes[0].Character = s.chars["char_006"]
	for i := range b.Heroes {
		b.Heroes[i].Acted = true
	}
	b.Heroes[0].Acted = false
	for i := range b.Enemies {
		b.Enemies[i].Blind = 1
	}
	raw, _ := json.Marshal(b)
	if _, err = db.Exec(`UPDATE rpg_battles SET state=? WHERE id=?`, string(raw), b.ID); err != nil {
		t.Fatal(err)
	}
	out, err = s.Act(ctx, p.ID, b.ID, Action{RequestID: "blind-passive-guard", Revision: b.Revision, Actor: 0, Target: 0, Action: "guard"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Battle.Heroes[0].HP != b.Heroes[0].HP || out.Battle.Heroes[0].PassiveBoost != .2 {
		t.Fatal("blind miss did not empower trickster")
	}
}
func TestMedicPassiveAddsEnergyBeyondActiveSkill(t *testing.T) {
	_, s, _ := fixture(t)
	b := passiveBattle("char_041")
	b.Heroes[0].Character = s.chars["char_041"]
	b.Heroes[0].Energy = 3
	if err := s.useSkill(b, 0, 0); err != nil {
		t.Fatal(err)
	}
	if b.Heroes[1].Energy != 3 || b.Heroes[0].PassiveRound != b.Round {
		t.Fatal("medic passive gave no additional energy")
	}
}
