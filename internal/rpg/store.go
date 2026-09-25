package rpg

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
)

type Service struct {
	db      *sqlx.DB
	mu      sync.Mutex
	catalog Catalog
	chars   map[string]Character
	now     func() time.Time
	random  io.Reader
}

func New(ctx context.Context, db *sqlx.DB) (*Service, error) {
	if db.DriverName() != "sqlite" {
		return nil, fmt.Errorf("RPG v1 requires DB_DRIVER=sqlite")
	}
	c, err := loadCatalog()
	if err != nil {
		return nil, err
	}
	s := &Service{db: db, catalog: c, chars: map[string]Character{}, now: time.Now, random: rand.Reader}
	for _, ch := range c.Characters {
		s.chars[ch.ID] = ch
	}
	if err = s.migrate(ctx); err != nil {
		return nil, err
	}
	return s, nil
}
func (s *Service) Catalog() Catalog { return s.catalog }
func (s *Service) migrate(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS rpg_migrations (version INTEGER PRIMARY KEY, applied_at BIGINT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS rpg_players (id TEXT PRIMARY KEY, state TEXT NOT NULL, revision BIGINT NOT NULL DEFAULT 0, created_at BIGINT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS rpg_identities (alias TEXT PRIMARY KEY, player_id TEXT NOT NULL REFERENCES rpg_players(id))`,
		`CREATE TABLE IF NOT EXISTS rpg_battles (id TEXT PRIMARY KEY, player_id TEXT NOT NULL REFERENCES rpg_players(id), status TEXT NOT NULL, revision BIGINT NOT NULL, state TEXT NOT NULL)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS rpg_one_active_battle ON rpg_battles(player_id) WHERE status='active'`,
		`CREATE TABLE IF NOT EXISTS rpg_requests (player_id TEXT NOT NULL REFERENCES rpg_players(id), request_id TEXT NOT NULL, payload_hash TEXT NOT NULL, response TEXT NOT NULL, created_at BIGINT NOT NULL, PRIMARY KEY(player_id,request_id))`,
		`CREATE INDEX IF NOT EXISTS rpg_requests_time ON rpg_requests(player_id,created_at)`,
		`CREATE TABLE IF NOT EXISTS rpg_ledger (player_id TEXT NOT NULL REFERENCES rpg_players(id), event_key TEXT NOT NULL, shards INTEGER NOT NULL, coins INTEGER NOT NULL, created_at BIGINT NOT NULL, PRIMARY KEY(player_id,event_key))`,
		`CREATE TABLE IF NOT EXISTS rpg_gacha_history (player_id TEXT NOT NULL REFERENCES rpg_players(id), request_id TEXT NOT NULL, results TEXT NOT NULL, created_at BIGINT NOT NULL, PRIMARY KEY(player_id,request_id))`,
		`CREATE TABLE IF NOT EXISTS rpg_tickets (token_hash TEXT PRIMARY KEY, player_id TEXT NOT NULL REFERENCES rpg_players(id), expires_at BIGINT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS rpg_ticket_owner ON rpg_tickets(player_id)`,
		`CREATE TABLE IF NOT EXISTS rpg_sessions (token_hash TEXT PRIMARY KEY, player_id TEXT NOT NULL REFERENCES rpg_players(id), expires_at BIGINT NOT NULL, created_at BIGINT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS rpg_session_owner ON rpg_sessions(player_id)`,
	} {
		if _, err = tx.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("RPG migration: %w", err)
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO rpg_migrations(version,applied_at) VALUES(1,?) ON CONFLICT(version) DO NOTHING`, s.now().Unix()); err != nil {
		return err
	}
	return tx.Commit()
}
func token() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}
func digest(raw string) string { sum := sha256.Sum256([]byte(raw)); return hex.EncodeToString(sum[:]) }

var requestPattern = regexp.MustCompile(`^[a-zA-Z0-9:_-]{8,160}$`)
var aliasPattern = regexp.MustCompile(`^[0-9]+@(s\.whatsapp\.net|lid)$`)

// EnsurePlayer is reachable only from the WhatsApp handler, never from HTTP.
// Trusted PN/LID aliases map to one internal ID so a different addressing mode
// does not create a second wallet. Conflicting existing accounts are not merged silently.
func (s *Service) EnsurePlayer(ctx context.Context, aliases []string, name string) (Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return Profile{}, err
	}
	defer tx.Rollback()
	if len(aliases) == 0 {
		return Profile{}, fail(400, "identity", "Identitas WhatsApp tidak tersedia.")
	}
	id := ""
	for _, alias := range aliases {
		if !aliasPattern.MatchString(alias) {
			return Profile{}, fail(400, "identity", "Identitas WhatsApp tidak valid.")
		}
		var found string
		err = tx.QueryRowContext(ctx, `SELECT player_id FROM rpg_identities WHERE alias=?`, alias).Scan(&found)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return Profile{}, err
		}
		if found != "" {
			if id != "" && id != found {
				return Profile{}, fail(409, "identity_conflict", "Dua profil RPG memiliki identitas berbeda. Hubungi pemilik bot.")
			}
			id = found
		}
	}
	var p Profile
	if id == "" {
		id, err = token()
		if err != nil {
			return p, err
		}
		name = strings.TrimSpace(name)
		runes := []rune(name)
		if len(runes) > 60 {
			name = string(runes[:60])
		}
		if name == "" {
			name = "Penjaga Fajar"
		}
		party := []string{"char_001", "char_002", "char_004", "char_023"}
		p = Profile{ID: id, Name: name, Version: 1, Shards: 1600, Party: party, Collection: map[string]int{}, Cleared: []int{}}
		for _, id := range party {
			p.Collection[id] = 1
		}
		raw, _ := json.Marshal(p)
		if _, err = tx.ExecContext(ctx, `INSERT INTO rpg_players(id,state,revision,created_at) VALUES(?,?,0,?)`, id, string(raw), s.now().Unix()); err != nil {
			return p, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO rpg_ledger(player_id,event_key,shards,coins,created_at) VALUES(?,'welcome',1600,0,?)`, id, s.now().Unix()); err != nil {
			return p, err
		}
	} else {
		p, err = loadPlayer(ctx, tx, id)
		if err != nil {
			return p, err
		}
	}
	for _, alias := range aliases {
		if _, err = tx.ExecContext(ctx, `INSERT INTO rpg_identities(alias,player_id) VALUES(?,?) ON CONFLICT(alias) DO NOTHING`, alias, id); err != nil {
			return p, err
		}
	}
	return p, tx.Commit()
}
func loadPlayer(ctx context.Context, tx *sqlx.Tx, id string) (Profile, error) {
	var raw string
	var p Profile
	err := tx.QueryRowContext(ctx, `SELECT state FROM rpg_players WHERE id=?`, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return p, fail(404, "profile_missing", "Profil RPG tidak ditemukan.")
	}
	if err != nil {
		return p, err
	}
	err = json.Unmarshal([]byte(raw), &p)
	return p, err
}
func loadBattle(ctx context.Context, tx *sqlx.Tx, player, id string) (*Battle, error) {
	if id == "" {
		return nil, nil
	}
	var raw string
	err := tx.QueryRowContext(ctx, `SELECT state FROM rpg_battles WHERE id=? AND player_id=?`, id, player).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fail(404, "battle_missing", "Battle tidak ditemukan.")
	}
	if err != nil {
		return nil, err
	}
	var b Battle
	if err = json.Unmarshal([]byte(raw), &b); err != nil {
		return nil, err
	}
	if b.Rules != RulesVersion {
		return nil, fail(409, "rules_changed", "Versi battle ini belum didukung server. Hubungi pemilik bot.")
	}
	return &b, nil
}
func (s *Service) Snapshot(ctx context.Context, id string) (Snapshot, error) {
	tx, err := s.db.BeginTxx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return Snapshot{}, err
	}
	defer tx.Rollback()
	p, err := loadPlayer(ctx, tx, id)
	if err != nil {
		return Snapshot{}, err
	}
	b, err := loadBattle(ctx, tx, id, p.LastBattle)
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{Profile: p, Battle: b}, tx.Commit()
}

type mutation func(*sqlx.Tx, *Profile) (*Battle, []Pull, error)

func (s *Service) mutate(ctx context.Context, player, requestID, kind string, payload any, fn mutation) (Snapshot, error) {
	if !requestPattern.MatchString(requestID) {
		return Snapshot{}, fail(400, "request_id", "Request ID tidak valid.")
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return Snapshot{}, err
	}
	hash := digest(kind + ":" + string(raw))
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return Snapshot{}, err
	}
	defer tx.Rollback()
	var oldHash, response string
	err = tx.QueryRowContext(ctx, `SELECT payload_hash,response FROM rpg_requests WHERE player_id=? AND request_id=?`, player, requestID).Scan(&oldHash, &response)
	if err == nil {
		if oldHash != hash {
			return Snapshot{}, fail(409, "request_reused", "Request ID sudah dipakai untuk aksi lain.")
		}
		var receipt struct {
			Results []Pull `json:"results,omitempty"`
		}
		if err := json.Unmarshal([]byte(response), &receipt); err != nil {
			return Snapshot{}, err
		}
		p, err := loadPlayer(ctx, tx, player)
		if err != nil {
			return Snapshot{}, err
		}
		b, err := loadBattle(ctx, tx, player, p.LastBattle)
		// Return the original draw results and latest account/battle state.
		// A receipt never rolls RNG or applies the mutation again.
		return Snapshot{Profile: p, Battle: b, Results: receipt.Results}, err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Snapshot{}, err
	}
	var recent int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM rpg_requests WHERE player_id=? AND created_at>?`, player, s.now().Add(-time.Minute).Unix()).Scan(&recent); err != nil {
		return Snapshot{}, err
	}
	if recent >= 180 {
		return Snapshot{}, fail(429, "rate_limited", "Terlalu banyak aksi. Tunggu sebentar.")
	}
	p, err := loadPlayer(ctx, tx, player)
	if err != nil {
		return Snapshot{}, err
	}
	oldRevision := p.Revision
	b, pulls, err := fn(tx, &p)
	if err != nil {
		return Snapshot{}, err
	}
	p.Revision++
	raw, err = json.Marshal(p)
	if err != nil {
		return Snapshot{}, err
	}
	res, err := tx.ExecContext(ctx, `UPDATE rpg_players SET state=?,revision=? WHERE id=? AND revision=?`, string(raw), p.Revision, player, oldRevision)
	if err != nil {
		return Snapshot{}, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return Snapshot{}, err
	}
	if n != 1 {
		return Snapshot{}, conflict()
	}
	if b == nil {
		b, err = loadBattle(ctx, tx, player, p.LastBattle)
		if err != nil {
			return Snapshot{}, err
		}
	}
	out := Snapshot{Profile: p, Battle: b, Results: pulls}
	// Keep a compact receipt rather than duplicating the entire battle and
	// profile for each click. The unique key is retained for replay protection.
	raw, err = json.Marshal(struct {
		Results []Pull `json:"results,omitempty"`
	}{pulls})
	if err != nil {
		return out, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO rpg_requests(player_id,request_id,payload_hash,response,created_at) VALUES(?,?,?,?,?)`, player, requestID, hash, string(raw), s.now().Unix()); err != nil {
		return Snapshot{}, err
	}
	if err = tx.Commit(); err != nil {
		return Snapshot{}, err
	}
	return out, nil
}
func saveBattle(ctx context.Context, tx *sqlx.Tx, player string, b *Battle, create bool) error {
	raw, err := json.Marshal(b)
	if err != nil {
		return err
	}
	status := "active"
	if b.Done {
		status = b.Result
	}
	if create {
		_, err = tx.ExecContext(ctx, `INSERT INTO rpg_battles(id,player_id,status,revision,state) VALUES(?,?,?,?,?)`, b.ID, player, status, b.Revision, string(raw))
		return err
	}
	res, err := tx.ExecContext(ctx, `UPDATE rpg_battles SET state=?,status=?,revision=? WHERE id=? AND player_id=? AND revision=?`, string(raw), status, b.Revision, b.ID, player, b.Revision-1)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return conflict()
	}
	return nil
}
