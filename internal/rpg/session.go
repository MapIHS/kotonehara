package rpg

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// IssueTicket invalidates older, unused links for this player. Raw tickets and
// session tokens never enter the database or application logs.
func (s *Service) IssueTicket(ctx context.Context, player string) (string, error) {
	raw, err := token()
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if _, err = loadPlayer(ctx, tx, player); err != nil {
		return "", err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM rpg_tickets WHERE player_id=? OR expires_at<=?`, player, s.now().Unix()); err != nil {
		return "", err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO rpg_tickets(token_hash,player_id,expires_at) VALUES(?,?,?)`, digest(raw), player, s.now().Add(2*time.Minute).Unix()); err != nil {
		return "", err
	}
	return raw, tx.Commit()
}
func (s *Service) ExchangeTicket(ctx context.Context, ticket string) (string, error) {
	if !tokenPattern.MatchString(ticket) {
		return "", fail(401, "ticket_invalid", "Link login tidak valid atau sudah kedaluwarsa. Minta .rpg lanjut di bot.")
	}
	raw, err := token()
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	now := s.now().Unix()
	var player string
	err = tx.QueryRowContext(ctx, `DELETE FROM rpg_tickets WHERE token_hash=? AND expires_at>? RETURNING player_id`, digest(ticket), now).Scan(&player)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fail(401, "ticket_invalid", "Link login sudah dipakai atau kedaluwarsa. Minta .rpg lanjut di bot.")
	}
	if err != nil {
		return "", err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM rpg_sessions WHERE expires_at<=?`, now); err != nil {
		return "", err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO rpg_sessions(token_hash,player_id,expires_at,created_at) VALUES(?,?,?,?)`, digest(raw), player, s.now().Add(7*24*time.Hour).Unix(), now); err != nil {
		return "", err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM rpg_sessions WHERE player_id=? AND token_hash NOT IN (SELECT token_hash FROM rpg_sessions WHERE player_id=? AND token_hash<>? ORDER BY created_at DESC,token_hash DESC LIMIT 9) AND token_hash<>?`, player, player, digest(raw), digest(raw)); err != nil {
		return "", err
	}
	return raw, tx.Commit()
}
func (s *Service) SessionPlayer(ctx context.Context, session string) (string, error) {
	if !tokenPattern.MatchString(session) {
		return "", fail(401, "session_required", "Buka link pribadi dari .rpg lanjut untuk masuk.")
	}
	var player string
	err := s.db.QueryRowContext(ctx, `SELECT player_id FROM rpg_sessions WHERE token_hash=? AND expires_at>?`, digest(session), s.now().Unix()).Scan(&player)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fail(401, "session_expired", "Sesi berakhir. Minta link baru melalui .rpg lanjut.")
	}
	return player, err
}
func (s *Service) Logout(ctx context.Context, session string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM rpg_sessions WHERE token_hash=?`, digest(session))
	return err
}
