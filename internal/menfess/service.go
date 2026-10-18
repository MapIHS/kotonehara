// Package menfess stores consent and routing for private anonymous conversations.
// Phone identities are internal routing data and must never be included in relay text.
package menfess

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jmoiron/sqlx"
)

var defaultService atomic.Pointer[Service]

func SetDefault(s *Service) { defaultService.Store(s) }
func Default() *Service     { return defaultService.Load() }

var (
	ErrSession  = errors.New("Sesi tidak ditemukan, sudah berakhir, atau bukan milikmu.")
	ErrBlocked  = errors.New("Undangan tidak dapat dikirim ke tujuan ini.")
	ErrBusy     = errors.New("Salah satu peserta masih memiliki undangan atau sesi Menfess. Selesaikan sesi sebelumnya dahulu.")
	ErrLimit    = errors.New("Batas undangan tercapai. Maksimal 3 per jam untuk pengirim dan 5 per hari untuk penerima.")
	ErrPending  = errors.New("Penerima belum menyetujui undangan Menfess.")
	ErrCooldown = errors.New("Tunggu 5 detik sebelum mengirim balasan berikutnya.")
)

var phoneJID = regexp.MustCompile(`^[1-9][0-9]{6,14}@s\.whatsapp\.net$`)
var codePattern = regexp.MustCompile(`^[a-f0-9]{12}$`)

const (
	InvitationTTL = 15 * time.Minute
	SessionTTL    = 24 * time.Hour
)

type Service struct {
	db  *sqlx.DB
	now func() time.Time
}
type Session struct {
	Code          string `db:"code"`
	Sender        string `db:"sender"`
	Recipient     string `db:"recipient"`
	Alias         string `db:"alias"`
	Initial       string `db:"initial_text"`
	State         string `db:"state"`
	Created       int64  `db:"created_at"`
	Expires       int64  `db:"expires_at"`
	LastSender    int64  `db:"last_sender"`
	LastRecipient int64  `db:"last_recipient"`
}

type Relay struct{ Code, To, Alias, Text, Notify string }

func New(ctx context.Context, db *sqlx.DB) (*Service, error) {
	if db == nil {
		return nil, errors.New("menfess: database tidak tersedia")
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	for _, query := range []string{
		`CREATE TABLE IF NOT EXISTS menfess_lock (id INTEGER PRIMARY KEY, value INTEGER NOT NULL)`,
		`INSERT INTO menfess_lock (id, value) VALUES (1, 0) ON CONFLICT (id) DO NOTHING`,
		`CREATE TABLE IF NOT EXISTS menfess_sessions (
   code TEXT PRIMARY KEY, sender TEXT NOT NULL, recipient TEXT NOT NULL,
   alias TEXT NOT NULL, initial_text TEXT NOT NULL, state TEXT NOT NULL,
   created_at BIGINT NOT NULL, expires_at BIGINT NOT NULL,
   last_sender BIGINT NOT NULL DEFAULT 0, last_recipient BIGINT NOT NULL DEFAULT 0
  )`,
		`CREATE INDEX IF NOT EXISTS menfess_sender_idx ON menfess_sessions (sender, created_at)`,
		`CREATE INDEX IF NOT EXISTS menfess_recipient_idx ON menfess_sessions (recipient, created_at)`,
		`CREATE TABLE IF NOT EXISTS menfess_preferences (jid TEXT PRIMARY KEY, blocked BOOLEAN NOT NULL)`,
	} {
		if _, err := tx.ExecContext(ctx, query); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &Service{db: db, now: time.Now}, nil
}

// Each operation takes a database write lock before reading state. This serializes
// invitations and consent across bot processes on SQLite and PostgreSQL.
func (s *Service) change(ctx context.Context, fn func(*sqlx.Tx, int64) error) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE menfess_lock SET value = 0 WHERE id = 1`); err != nil {
		return err
	}
	now := s.now().Unix()
	// Expiry is enforced on every operation, even if no background worker runs.
	if _, err := tx.ExecContext(ctx, s.db.Rebind(`UPDATE menfess_sessions SET state = 'expired', initial_text = '', alias = '' WHERE expires_at <= ? AND state IN ('pending', 'active')`), now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, s.db.Rebind(`DELETE FROM menfess_sessions WHERE created_at < ? AND state NOT IN ('pending', 'active')`), now-int64((48*time.Hour).Seconds())); err != nil {
		return err
	}
	// Commit housekeeping even if the requested operation is denied.
	opErr := fn(tx, now)
	if opErr != nil && !IsRejection(opErr) {
		return opErr
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return opErr
}

func ValidateText(text string) error {
	if strings.TrimSpace(text) == "" || !utf8.ValidString(text) || utf8.RuneCountInString(text) > 1000 || strings.IndexFunc(text, func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\t' }) >= 0 {
		return errors.New("Pesan harus berisi 1–1000 karakter teks.")
	}
	return nil
}

func (s *Service) Invite(ctx context.Context, sender, recipient, alias, text string) (Session, error) {
	var session Session
	alias, text = strings.TrimSpace(alias), strings.TrimSpace(text)
	if !phoneJID.MatchString(sender) || !phoneJID.MatchString(recipient) || sender == recipient {
		return session, errors.New("Nomor tujuan tidak valid atau sama dengan pengirim.")
	}
	if err := ValidateAlias(alias); err != nil {
		return session, err
	}
	if err := ValidateText(text); err != nil {
		return session, err
	}
	var token [6]byte
	if _, err := rand.Read(token[:]); err != nil {
		return session, err
	}
	session = Session{Code: hex.EncodeToString(token[:]), Sender: sender, Recipient: recipient, Alias: alias, Initial: text, State: "pending"}
	err := s.change(ctx, func(tx *sqlx.Tx, now int64) error {
		var blocked bool
		err := tx.GetContext(ctx, &blocked, s.db.Rebind(`SELECT blocked FROM menfess_preferences WHERE jid = ?`), recipient)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if blocked {
			return ErrBlocked
		}
		var count int
		if err := tx.GetContext(ctx, &count, s.db.Rebind(`SELECT COUNT(*) FROM menfess_sessions WHERE state IN ('pending', 'active') AND (sender IN (?, ?) OR recipient IN (?, ?))`), sender, recipient, sender, recipient); err != nil {
			return err
		}
		if count > 0 {
			return ErrBusy
		}
		if err := tx.GetContext(ctx, &count, s.db.Rebind(`SELECT COUNT(*) FROM menfess_sessions WHERE sender = ? AND created_at > ?`), sender, now-3600); err != nil {
			return err
		}
		if count >= 3 {
			return ErrLimit
		}
		if err := tx.GetContext(ctx, &count, s.db.Rebind(`SELECT COUNT(*) FROM menfess_sessions WHERE recipient = ? AND created_at > ?`), recipient, now-86400); err != nil {
			return err
		}
		if count >= 5 {
			return ErrLimit
		}
		session.Created, session.Expires = now, now+int64(InvitationTTL.Seconds())
		_, err = tx.NamedExecContext(ctx, `INSERT INTO menfess_sessions (code, sender, recipient, alias, initial_text, state, created_at, expires_at) VALUES (:code, :sender, :recipient, :alias, :initial_text, :state, :created_at, :expires_at)`, session)
		return err
	})
	return session, err
}

func (s *Service) find(ctx context.Context, tx *sqlx.Tx, actor, code string) (Session, error) {
	var session Session
	if !phoneJID.MatchString(actor) {
		return session, ErrSession
	}
	code = strings.ToLower(strings.TrimSpace(code))
	if code != "" && !codePattern.MatchString(code) {
		return session, ErrSession
	}
	query := `SELECT * FROM menfess_sessions WHERE state IN ('pending', 'active') AND (sender = ? OR recipient = ?)`
	args := []any{actor, actor}
	if code != "" {
		query += ` AND code = ?`
		args = append(args, code)
	}
	err := tx.GetContext(ctx, &session, s.db.Rebind(query), args...)
	if errors.Is(err, sql.ErrNoRows) {
		return session, ErrSession
	}
	return session, err
}

// Accept is recipient-only and releases the initial message once. Message text
// is removed from storage after acceptance; subsequent replies are never stored.
func (s *Service) Accept(ctx context.Context, actor, code string) (Relay, error) {
	var relay Relay
	if strings.TrimSpace(code) == "" {
		return relay, ErrSession
	}
	err := s.change(ctx, func(tx *sqlx.Tx, now int64) error {
		session, err := s.find(ctx, tx, actor, code)
		if err != nil {
			return err
		}
		if session.Recipient != actor || session.State != "pending" {
			return ErrSession
		}
		relay = Relay{Code: session.Code, To: actor, Alias: session.Alias, Text: session.Initial, Notify: session.Sender}
		_, err = tx.ExecContext(ctx, s.db.Rebind(`UPDATE menfess_sessions SET state = 'active', initial_text = '', expires_at = ?, last_sender = ? WHERE code = ?`), now+int64(SessionTTL.Seconds()), now, session.Code)
		return err
	})
	return relay, err
}

func (s *Service) Reply(ctx context.Context, actor, code, text string) (Relay, error) {
	var relay Relay
	if strings.TrimSpace(code) == "" {
		return relay, ErrSession
	}
	text = strings.TrimSpace(text)
	if err := ValidateText(text); err != nil {
		return relay, err
	}
	err := s.change(ctx, func(tx *sqlx.Tx, now int64) error {
		session, err := s.find(ctx, tx, actor, code)
		if err != nil {
			return err
		}
		if session.State != "active" {
			return ErrPending
		}
		relay = Relay{Code: session.Code, To: session.Sender, Alias: "Penerima", Text: text}
		last, column := session.LastRecipient, "last_recipient"
		if actor == session.Sender {
			relay.To, relay.Alias, last, column = session.Recipient, session.Alias, session.LastSender, "last_sender"
		}
		if last > 0 && now-last < 5 {
			return ErrCooldown
		}
		_, err = tx.ExecContext(ctx, s.db.Rebind(fmt.Sprintf(`UPDATE menfess_sessions SET %s = ? WHERE code = ?`, column)), now, session.Code)
		return err
	})
	return relay, err
}

func (s *Service) Stop(ctx context.Context, actor, code string) error {
	return s.change(ctx, func(tx *sqlx.Tx, _ int64) error {
		session, err := s.find(ctx, tx, actor, code)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, s.db.Rebind(`UPDATE menfess_sessions SET state = 'closed', initial_text = '', alias = '' WHERE code = ?`), session.Code)
		return err
	})
}

// Blocking closes existing invitations/conversations and rejects all new invites.
func (s *Service) Block(ctx context.Context, actor string, blocked bool) error {
	if !phoneJID.MatchString(actor) {
		return ErrSession
	}
	return s.change(ctx, func(tx *sqlx.Tx, _ int64) error {
		if _, err := tx.ExecContext(ctx, s.db.Rebind(`INSERT INTO menfess_preferences (jid, blocked) VALUES (?, ?) ON CONFLICT (jid) DO UPDATE SET blocked = EXCLUDED.blocked`), actor, blocked); err != nil {
			return err
		}
		if !blocked {
			return nil
		}
		_, err := tx.ExecContext(ctx, s.db.Rebind(`UPDATE menfess_sessions SET state = 'closed', initial_text = '', alias = '' WHERE (sender = ? OR recipient = ?) AND state IN ('pending', 'active')`), actor, actor)
		return err
	})
}

// IsRejection distinguishes expected refusals from storage failures.
func IsRejection(err error) bool {
	for _, rejection := range []error{ErrSession, ErrBlocked, ErrBusy, ErrLimit, ErrPending, ErrCooldown} {
		if errors.Is(err, rejection) {
			return true
		}
	}
	return false
}

func (s *Service) Prune(ctx context.Context) error {
	return s.change(ctx, func(*sqlx.Tx, int64) error { return nil })
}

func ValidateAlias(alias string) error {
	if strings.TrimSpace(alias) == "" || utf8.RuneCountInString(alias) > 40 || !utf8.ValidString(alias) || strings.IndexFunc(alias, unicode.IsControl) >= 0 {
		return errors.New("Nama samaran harus 1–40 karakter dalam satu baris.")
	}
	return nil
}
