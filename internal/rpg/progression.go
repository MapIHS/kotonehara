package rpg

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
)

type Growth struct {
	Level int `json:"level"`
	XP    int `json:"xp"`
}

type Equipment struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Slot    string `json:"slot"`
	Price   int    `json:"price"`
	Unlock  int    `json:"unlock"`
	HP      int    `json:"hp"`
	Attack  int    `json:"attack"`
	Defense int    `json:"defense"`
}

var equipmentCatalog = []Equipment{
	{"blade_dawn", "Bilah Fajar", "weapon", 30, 0, 0, 8, 0},
	{"coat_moss", "Mantel Lumut", "armor", 30, 0, 40, 0, 4},
	{"charm_dew", "Jimat Embun", "charm", 40, 0, 20, 3, 3},
	{"blade_lantern", "Bilah Lentera", "weapon", 600, 99, 0, 24, 0},
	{"coat_coral", "Zirah Karang", "armor", 600, 99, 150, 0, 16},
	{"charm_echo", "Jimat Gema", "charm", 800, 99, 70, 10, 10},
	{"blade_aurora", "Bilah Aurora", "weapon", 2400, 399, 0, 60, 0},
	{"coat_sun", "Zirah Surya", "armor", 2400, 399, 400, 0, 40},
	{"charm_origin", "Jimat Arunika", "charm", 3200, 399, 200, 25, 25},
}

// Lazy, additive profile upgrade. Old wallets, collections and battles are kept.
// Existing characters retain the level they previously had through stage scaling.
func normalizeProgress(p *Profile) {
	if p.Growth == nil {
		p.Growth = map[string]Growth{}
	}
	if p.Inventory == nil {
		p.Inventory = map[string]int{}
	}
	if p.Loadouts == nil {
		p.Loadouts = map[string]map[string]string{}
	}
	p.LevelCap = min(999, max(10, p.Unlocked+1))
	for id, count := range p.Collection {
		if count > 0 && p.Growth[id].Level == 0 {
			p.Growth[id] = Growth{Level: min(999, max(1, p.Unlocked+1))}
		}
	}
	p.Version = 2
}

func grantXP(p *Profile, id string, amount int) {
	g := p.Growth[id]
	g.XP += amount
	for g.XP >= 100 && g.Level < p.LevelCap {
		g.Level++
		g.XP -= 100
	}
	if g.Level >= p.LevelCap {
		g.XP = min(g.XP, 99)
	}
	if g.Level == 999 {
		g.XP = 0
	}
	p.Growth[id] = g
}

func findEquipment(id string) (Equipment, bool) {
	for _, item := range equipmentCatalog {
		if item.ID == id {
			return item, true
		}
	}
	return Equipment{}, false
}

type ProgressRequest struct {
	RequestID   string `json:"request_id"`
	Revision    int64  `json:"expected_revision"`
	CharacterID string `json:"character_id"`
	Levels      int    `json:"levels,omitempty"`
	ItemID      string `json:"item_id,omitempty"`
	Slot        string `json:"slot,omitempty"`
}

func (s *Service) Progress(ctx context.Context, player, operation string, a ProgressRequest) (Snapshot, error) {
	return s.mutate(ctx, player, a.RequestID, "progress:"+operation, a, func(tx *sqlx.Tx, p *Profile) (*Battle, []Pull, error) {
		if p.Revision != a.Revision {
			return nil, nil, conflict()
		}
		b, err := loadBattle(ctx, tx, player, p.LastBattle)
		if err != nil {
			return nil, nil, err
		}
		if b != nil && !b.Done {
			return nil, nil, fail(409, "battle_active", "Selesaikan atau mundur dari battle sebelum mengatur latihan dan equipment.")
		}
		if operation != "buy" && p.Collection[a.CharacterID] < 1 {
			return nil, nil, fail(403, "character_unowned", "Karakter belum dimiliki.")
		}
		cost := 0
		switch operation {
		case "train":
			if a.Levels != 1 && a.Levels != 10 {
				return nil, nil, fail(400, "levels", "Pilih latihan satu atau sepuluh level.")
			}
			g := p.Growth[a.CharacterID]
			if g.Level+a.Levels > p.LevelCap {
				return nil, nil, fail(409, "level_cap", "Batas level tercapai. Lanjutkan cerita untuk menaikkannya.")
			}
			cost = 10 * a.Levels
			if p.TrainingXP < 100*a.Levels || p.Coins < cost {
				return nil, nil, fail(409, "funds", "XP latihan atau koin belum cukup.")
			}
			p.TrainingXP -= 100 * a.Levels
			g.Level += a.Levels
			if g.Level == 999 {
				g.XP = 0
			}
			p.Growth[a.CharacterID] = g
		case "buy":
			item, ok := findEquipment(a.ItemID)
			if !ok {
				return nil, nil, fail(400, "equipment", "Equipment tidak ditemukan.")
			}
			if p.Unlocked < item.Unlock {
				return nil, nil, fail(403, "equipment_locked", "Lanjutkan cerita untuk membuka equipment ini.")
			}
			cost = item.Price
			if p.Coins < cost {
				return nil, nil, fail(409, "funds", "Koin belum cukup.")
			}
			if p.Inventory[item.ID] >= 60 {
				return nil, nil, fail(409, "inventory_full", "Salinan equipment ini sudah cukup untuk seluruh koleksi.")
			}
			p.Inventory[item.ID]++
		case "equip":
			if a.Slot != "weapon" && a.Slot != "armor" && a.Slot != "charm" {
				return nil, nil, fail(400, "slot", "Slot equipment tidak valid.")
			}
			if a.ItemID != "" {
				item, ok := findEquipment(a.ItemID)
				if !ok || item.Slot != a.Slot {
					return nil, nil, fail(400, "equipment", "Equipment tidak cocok dengan slot.")
				}
				used := 0
				for id, slots := range p.Loadouts {
					if id != a.CharacterID && slots[a.Slot] == a.ItemID {
						used++
					}
				}
				if p.Inventory[a.ItemID] <= used {
					return nil, nil, fail(409, "equipment_unavailable", "Equipment belum dimiliki atau sudah dipakai karakter lain.")
				}
			}
			if p.Loadouts[a.CharacterID] == nil {
				p.Loadouts[a.CharacterID] = map[string]string{}
			}
			p.Loadouts[a.CharacterID][a.Slot] = a.ItemID
		default:
			return nil, nil, fail(400, "operation", "Pengaturan tidak dikenal.")
		}
		p.Coins -= cost
		if cost > 0 {
			_, err = tx.ExecContext(ctx, `INSERT INTO rpg_ledger(player_id,event_key,shards,coins,created_at) VALUES(?,?,0,?,?)`, p.ID, fmt.Sprintf("%s:%s", operation, a.RequestID), -cost, s.now().Unix())
		}
		return b, nil, err
	})
}
