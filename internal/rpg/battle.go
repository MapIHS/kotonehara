package rpg

import (
	"context"
	"crypto/rand"
	"fmt"
	"math"
	"math/big"
	"slices"

	"github.com/jmoiron/sqlx"
)

func (s *Service) roll(n int) (int, error) {
	v, err := rand.Int(s.random, big.NewInt(int64(n)))
	if err != nil {
		return 0, err
	}
	return int(v.Int64()), nil
}
func rounded(n float64) int { return max(1, int(math.Round(n))) }
func (s *Service) StartBattle(ctx context.Context, player, request string, stage int) (Snapshot, error) {
	return s.mutate(ctx, player, request, "start", stage, func(tx *sqlx.Tx, p *Profile) (*Battle, []Pull, error) {
		if stage < 0 || stage >= len(s.catalog.Stages) || stage > p.Unlocked {
			return nil, nil, fail(403, "stage_locked", "Lokasi ini belum terbuka.")
		}
		current, err := loadBattle(ctx, tx, player, p.LastBattle)
		if err != nil {
			return nil, nil, err
		}
		if current != nil && !current.Done {
			return nil, nil, fail(409, "battle_active", "Lanjutkan atau mundur dari battle yang sedang aktif.")
		}
		id, err := token()
		if err != nil {
			return nil, nil, err
		}
		st := s.catalog.Stages[stage]
		level := float64(st.Level)
		b := &Battle{ID: id, Rules: RulesVersion, Stage: stage, Round: 1, Phase: "player", Heroes: []Hero{}, Enemies: []Opponent{}, Logs: []string{}}
		bases := map[string][3]float64{"Vanguard": {220, 42, 26}, "Guardian": {290, 29, 42}, "Ranger": {185, 45, 20}, "Arcanist": {175, 49, 18}, "Medic": {205, 28, 25}, "Trickster": {190, 38, 22}}
		for _, id := range p.Party {
			c := s.chars[id]
			base := bases[c.Role]
			rarity := []float64{1, 1.04, 1.08, 1.14, 1.20}[c.Rarity-1]
			hp := rounded(base[0] * (1 + (level-1)*.022) * rarity)
			// Art and design prose are served once in the catalog, not copied into every battle snapshot.
			c.Art = Art{}
			c.Visual = ""
			c.Personality = ""
			c.Passive = Ability{}
			c.Skill.Description = ""
			b.Heroes = append(b.Heroes, Hero{Character: c, HP: hp, Max: hp, Attack: rounded(base[1] * (1 + (level-1)*.0105) * rarity), Defense: rounded(base[2] * (1 + (level-1)*.008) * rarity), Energy: 3})
		}
		var species Enemy
		for _, e := range s.catalog.Enemies {
			if e.MinLevel <= st.Level && e.MaxLevel >= st.Level {
				species = e
				break
			}
		}
		if species.ID == "" {
			return nil, nil, fmt.Errorf("enemy missing at level %d", st.Level)
		}
		species.Art = Art{}
		species.Visual = ""
		count, hpFactor, atkFactor, defFactor := 2, 1.0, 1.0, 1.0
		if st.Elite {
			hpFactor, atkFactor, defFactor = 1.8, 1.35, 1.2
		}
		if st.Boss {
			count, hpFactor, atkFactor, defFactor = 1, 3.2, 1.7, 1.45
		}
		for i := 0; i < count; i++ {
			e := species
			if i > 0 {
				e.Name += " · B"
			}
			kind := "moth"
			if e.Archetype == "slime" {
				kind = "slime"
			}
			if st.Boss {
				kind = "boss"
			}
			hp := rounded((110 + float64(i)*15) * (1 + (level-1)*.022) * hpFactor)
			b.Enemies = append(b.Enemies, Opponent{Enemy: e, HP: hp, Max: hp, Attack: rounded(28 * (1 + (level-1)*.0105) * atkFactor), Defense: rounded(14 * (1 + (level-1)*.008) * defFactor), Kind: kind})
		}
		b.log("Pilih target dan aksi. Setiap anggota tim bertindak sekali per ronde.")
		p.LastBattle = id
		return b, nil, saveBattle(ctx, tx, player, b, true)
	})
}
func (s *Service) Act(ctx context.Context, player, battleID string, a Action) (Snapshot, error) {
	return s.mutate(ctx, player, a.RequestID, "action:"+battleID, a, func(tx *sqlx.Tx, p *Profile) (*Battle, []Pull, error) {
		b, err := loadBattle(ctx, tx, player, battleID)
		if err != nil {
			return nil, nil, err
		}
		if b == nil {
			return nil, nil, fail(404, "battle_missing", "Battle tidak ditemukan.")
		}
		if b.Revision != a.Revision {
			return nil, nil, conflict()
		}
		if b.Done {
			return nil, nil, fail(409, "battle_done", "Battle sudah selesai.")
		}
		if a.Actor < 0 || a.Actor >= len(b.Heroes) || a.Target < 0 || a.Target >= len(b.Enemies) {
			return nil, nil, fail(400, "invalid_target", "Karakter atau target tidak valid.")
		}
		h := &b.Heroes[a.Actor]
		target := &b.Enemies[a.Target]
		if h.HP <= 0 || h.Acted {
			return nil, nil, fail(409, "actor_unavailable", "Karakter ini tidak dapat bertindak.")
		}
		if !slices.Contains([]string{"attack", "skill", "guard", "ultimate"}, a.Action) {
			return nil, nil, fail(400, "invalid_action", "Aksi tidak dikenal.")
		}
		if (a.Action == "skill" && h.Energy < 2) || (a.Action == "ultimate" && h.Energy < 5) {
			return nil, nil, fail(409, "energy", "Energi tidak cukup.")
		}
		offensive := a.Action == "attack" || (a.Action == "skill" && h.Role != "Medic" && h.Role != "Guardian")
		if offensive && target.HP <= 0 {
			return nil, nil, fail(409, "target_down", "Target sudah dikalahkan.")
		}
		h.Acted = true
		b.Target = a.Target
		switch a.Action {
		case "guard":
			h.Guard = true
			h.Energy = min(5, h.Energy+2)
			b.log(h.Name + " bertahan sampai fase musuh berakhir.")
		case "ultimate":
			h.Energy = 0
			if h.Role == "Medic" {
				for i := range b.Heroes {
					ally := &b.Heroes[i]
					if ally.HP > 0 {
						ally.HP = min(ally.Max, ally.HP+rounded(float64(ally.Max)*.55))
					}
				}
				b.log(h.Name + " memulihkan seluruh tim.")
			} else {
				for i := range b.Enemies {
					e := &b.Enemies[i]
					if e.HP <= 0 {
						continue
					}
					n, err := s.hit(h.Attack, e.Defense, b.Stage+1, 2.8, h.Element, e.Element, false)
					if err != nil {
						return nil, nil, err
					}
					e.HP = max(0, e.HP-n)
				}
				b.log(h.Name + " melepaskan Ultimate ke semua musuh.")
			}
		default:
			if a.Action == "skill" && h.Role == "Medic" {
				h.Energy -= 2
				index := -1
				for i, v := range b.Heroes {
					if v.HP > 0 && (index < 0 || float64(v.HP)/float64(v.Max) < float64(b.Heroes[index].HP)/float64(b.Heroes[index].Max)) {
						index = i
					}
				}
				ally := &b.Heroes[index]
				heal := min(ally.Max-ally.HP, rounded(float64(ally.Max)*.38))
				ally.HP += heal
				b.log(fmt.Sprintf("%s memulihkan %s: +%d HP.", h.Name, ally.Name, heal))
			} else if a.Action == "skill" && h.Role == "Guardian" {
				h.Energy -= 2
				for i := range b.Heroes {
					if b.Heroes[i].HP > 0 {
						b.Heroes[i].Guard = true
					}
				}
				b.log(h.Name + " melindungi seluruh tim sampai fase musuh berakhir.")
			} else {
				coefficient := 1.0
				verb := "menyerang"
				if a.Action == "skill" {
					h.Energy -= 2
					coefficient = 1.85
					verb = "menyerang kuat"
				} else {
					h.Energy = min(5, h.Energy+1)
				}
				n, err := s.hit(h.Attack, target.Defense, b.Stage+1, coefficient, h.Element, target.Element, false)
				if err != nil {
					return nil, nil, err
				}
				target.HP = max(0, target.HP-n)
				b.log(fmt.Sprintf("%s %s → %s: %d damage.", h.Name, verb, target.Name, n))
			}
		}
		if livingEnemies(b) == 0 {
			if err = s.complete(ctx, tx, p, b, "win"); err != nil {
				return nil, nil, err
			}
		} else {
			if b.Enemies[b.Target].HP <= 0 {
				for i, e := range b.Enemies {
					if e.HP > 0 {
						b.Target = i
						break
					}
				}
			}
			next := nextHero(b)
			if next < 0 {
				for i := range b.Enemies {
					e := &b.Enemies[i]
					if e.HP <= 0 {
						continue
					}
					living := []int{}
					for j, h := range b.Heroes {
						if h.HP > 0 {
							living = append(living, j)
						}
					}
					roll, err := s.roll(len(living))
					if err != nil {
						return nil, nil, err
					}
					ally := &b.Heroes[living[roll]]
					factor := 1.0
					if e.Charged {
						factor = 1.7
					}
					n, err := s.hit(e.Attack, ally.Defense, b.Stage+1, factor, e.Element, ally.Element, ally.Guard)
					if err != nil {
						return nil, nil, err
					}
					ally.HP = max(0, ally.HP-n)
					b.log(fmt.Sprintf("%s menyerang %s: %d damage.", e.Name, ally.Name, n))
					e.Charged = (b.Round+1)%3 == 0
					alive := false
					for _, h := range b.Heroes {
						if h.HP > 0 {
							alive = true
						}
					}
					if !alive {
						if err = s.complete(ctx, tx, p, b, "lose"); err != nil {
							return nil, nil, err
						}
						break
					}
				}
				if !b.Done {
					b.Round++
					for i := range b.Heroes {
						b.Heroes[i].Acted = false
						b.Heroes[i].Guard = false
					}
					next = nextHero(b)
				}
			}
			if next >= 0 {
				b.Active = next
			}
		}
		b.Revision++
		return b, nil, saveBattle(ctx, tx, player, b, false)
	})
}
func nextHero(b *Battle) int {
	for i, h := range b.Heroes {
		if h.HP > 0 && !h.Acted {
			return i
		}
	}
	return -1
}
func livingEnemies(b *Battle) int {
	n := 0
	for _, e := range b.Enemies {
		if e.HP > 0 {
			n++
		}
	}
	return n
}
func elementFactor(attack, defense string) float64 {
	if (attack == "Cahaya" && defense == "Bayangan") || (attack == "Bayangan" && defense == "Cahaya") {
		return 1.2
	}
	cycle := map[string]string{"Api": "Angin", "Angin": "Tanah", "Tanah": "Air", "Air": "Api"}
	if cycle[attack] == defense {
		return 1.2
	}
	if cycle[defense] == attack {
		return .85
	}
	return 1
}
func (s *Service) hit(atk, def, level int, coefficient float64, element, target string, guard bool) (int, error) {
	v, err := s.roll(101)
	if err != nil {
		return 0, err
	}
	critical, err := s.roll(100)
	if err != nil {
		return 0, err
	}
	scale := 100 * (1 + float64(level-1)*.008)
	factor := 1.0
	if guard {
		factor = .5
	}
	if critical < 5 {
		factor *= 1.5
	}
	return max(1, int(math.Floor(float64(atk)*coefficient*scale/(scale+float64(def))*elementFactor(element, target)*(.95+float64(v)/1000)*factor))), nil
}
func (s *Service) complete(ctx context.Context, tx *sqlx.Tx, p *Profile, b *Battle, result string) error {
	b.Done = true
	b.Result = result
	b.Phase = "complete"
	if result == "win" {
		b.FirstClear = !slices.Contains(p.Cleared, b.Stage)
		if b.FirstClear {
			p.Cleared = append(p.Cleared, b.Stage)
			p.Unlocked = min(998, max(p.Unlocked, b.Stage+1))
			b.RewardShards = 20
			b.RewardCoins = 30 + b.Stage
			if s.catalog.Stages[b.Stage].Boss {
				b.RewardShards = 200
				b.RewardCoins *= 3
			}
			p.Shards += b.RewardShards
			p.Coins += b.RewardCoins
			if _, err := tx.ExecContext(ctx, `INSERT INTO rpg_ledger(player_id,event_key,shards,coins,created_at) VALUES(?,?,?,?,?)`, p.ID, fmt.Sprintf("clear:%d", b.Stage), b.RewardShards, b.RewardCoins, s.now().Unix()); err != nil {
				return err
			}
			b.log(fmt.Sprintf("Menang! +%d Embun Bintang · +%d koin. Progres tersimpan.", b.RewardShards, b.RewardCoins))
		} else {
			b.log("Menang latihan. Hadiah pertama sudah pernah diambil.")
		}
	} else if result == "lose" {
		b.log("Tim kalah. Coba susunan aksi lain; tidak ada hadiah.")
	} else {
		b.log("Kembali ke perkemahan tanpa hadiah.")
	}
	return nil
}
func (s *Service) Retreat(ctx context.Context, player, battleID, request string, revision int64) (Snapshot, error) {
	return s.mutate(ctx, player, request, "retreat:"+battleID, revision, func(tx *sqlx.Tx, p *Profile) (*Battle, []Pull, error) {
		b, err := loadBattle(ctx, tx, player, battleID)
		if err != nil {
			return nil, nil, err
		}
		if b == nil {
			return nil, nil, fail(404, "battle_missing", "Battle tidak ditemukan.")
		}
		if b.Revision != revision {
			return nil, nil, conflict()
		}
		if b.Done {
			return nil, nil, fail(409, "battle_done", "Battle sudah selesai.")
		}
		if err = s.complete(ctx, tx, p, b, "retreat"); err != nil {
			return nil, nil, err
		}
		b.Revision++
		return b, nil, saveBattle(ctx, tx, player, b, false)
	})
}
func (s *Service) SetParty(ctx context.Context, player string, a PartyRequest) (Snapshot, error) {
	return s.mutate(ctx, player, a.RequestID, "party", a, func(tx *sqlx.Tx, p *Profile) (*Battle, []Pull, error) {
		if a.Revision != p.Revision {
			return nil, nil, conflict()
		}
		if len(a.Party) != 4 {
			return nil, nil, fail(400, "party", "Tim harus berisi empat karakter.")
		}
		seen := map[string]bool{}
		for _, id := range a.Party {
			if _, ok := s.chars[id]; !ok || p.Collection[id] < 1 || seen[id] {
				return nil, nil, fail(400, "party", "Karakter harus dimiliki dan tidak boleh duplikat dalam tim.")
			}
			seen[id] = true
		}
		b, err := loadBattle(ctx, tx, player, p.LastBattle)
		if err != nil {
			return nil, nil, err
		}
		if b != nil && !b.Done {
			return nil, nil, fail(409, "battle_active", "Selesaikan atau mundur dari battle sebelum mengganti tim.")
		}
		p.Party = append([]string{}, a.Party...)
		return b, nil, nil
	})
}
