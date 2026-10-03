package rpg

import (
	"context"
	"encoding/json"
	"github.com/jmoiron/sqlx"
)

const BannerID = "fajar-v1"

func (s *Service) Summon(ctx context.Context, player, request, banner string, count int) (Snapshot, error) {
	payload := struct {
		Banner string
		Count  int
	}{banner, count}
	return s.mutate(ctx, player, request, "gacha", payload, func(tx *sqlx.Tx, p *Profile) (*Battle, []Pull, error) {
		if banner != BannerID {
			return nil, nil, fail(400, "banner", "Banner tidak tersedia.")
		}
		if count != 1 && count != 10 {
			return nil, nil, fail(400, "count", "Pilih satu atau sepuluh tarikan.")
		}
		if p.Shards < count*160 {
			return nil, nil, fail(409, "funds", "Embun Bintang tidak cukup.")
		}
		p.Shards -= count * 160
		pulls := make([]Pull, 0, count)
		for range count {
			result, err := s.pull(p)
			if err != nil {
				return nil, nil, err
			}
			pulls = append(pulls, result)
		}
		raw, err := json.Marshal(pulls)
		if err != nil {
			return nil, nil, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO rpg_gacha_history(player_id,request_id,results,created_at) VALUES(?,?,?,?)`, player, request, string(raw), s.now().Unix()); err != nil {
			return nil, nil, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO rpg_ledger(player_id,event_key,shards,coins,created_at) VALUES(?,?,?,0,?)`, player, "gacha:"+request, -count*160, s.now().Unix()); err != nil {
			return nil, nil, err
		}
		return nil, pulls, nil
	})
}
func (s *Service) pull(p *Profile) (Pull, error) {
	next5, next4 := p.Pity5+1, p.Pity4+1
	chance5 := min(10000, 200+max(0, next5-60)*500)
	if next5 >= 80 {
		chance5 = 10000
	}
	roll, err := s.roll(10000)
	if err != nil {
		return Pull{}, err
	}
	rarity := 4
	if roll < chance5 {
		rarity = 5
	} else if next4 < 10 {
		v, err := s.roll(98)
		if err != nil {
			return Pull{}, err
		}
		for i, w := range []int{40, 30, 20, 8} {
			v -= w
			if v < 0 {
				rarity = i + 1
				break
			}
		}
	}
	var selected Character
	if rarity == 5 {
		featured := p.Guarantee
		if !featured {
			v, err := s.roll(2)
			if err != nil {
				return Pull{}, err
			}
			featured = v == 0
		}
		if featured {
			selected = s.chars[s.catalog.Featured]
			p.Guarantee = false
		} else {
			pool := []Character{}
			for _, c := range s.catalog.Characters {
				if c.Rarity == 5 && c.ID != s.catalog.Featured {
					pool = append(pool, c)
				}
			}
			v, err := s.roll(len(pool))
			if err != nil {
				return Pull{}, err
			}
			selected = pool[v]
			p.Guarantee = true
		}
		p.Pity5 = 0
		p.Pity4 = 0
	} else {
		pool := []Character{}
		for _, c := range s.catalog.Characters {
			if c.Rarity == rarity {
				pool = append(pool, c)
			}
		}
		v, err := s.roll(len(pool))
		if err != nil {
			return Pull{}, err
		}
		selected = pool[v]
		p.Pity5++
		if rarity == 4 {
			p.Pity4 = 0
		} else {
			p.Pity4++
		}
	}
	duplicate := p.Collection[selected.ID] > 0
	p.Collection[selected.ID]++
	if duplicate {
		p.Dust += []int{0, 5, 10, 20, 40, 80}[rarity]
	}
	return Pull{Character: selected, Duplicate: duplicate}, nil
}

type HistoryEntry struct {
	RequestID string `json:"request_id"`
	CreatedAt int64  `json:"created_at"`
	Results   []Pull `json:"results"`
}

func (s *Service) History(ctx context.Context, player string) ([]HistoryEntry, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT request_id,created_at,results FROM rpg_gacha_history WHERE player_id=? ORDER BY created_at DESC,request_id DESC LIMIT 20`, player)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []HistoryEntry{}
	for rows.Next() {
		var h HistoryEntry
		var raw string
		if err = rows.Scan(&h.RequestID, &h.CreatedAt, &raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &h.Results); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
