package menfess

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

const (
	alice = "628123456789@s.whatsapp.net"
	bob   = "628987654321@s.whatsapp.net"
	eve   = "628111111111@s.whatsapp.net"
)

func newTestService(t *testing.T) (*Service, *time.Time) {
	t.Helper()
	db, err := sqlx.Open("sqlite", filepath.Join(t.TempDir(), "menfess.db")+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	s, err := New(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1800000000, 0)
	s.now = func() time.Time { return now }
	return s, &now
}

func invite(t *testing.T, s *Service) Session {
	t.Helper()
	session, err := s.Invite(context.Background(), alice, bob, "Bintang", "Halo rahasia")
	if err != nil {
		t.Fatal(err)
	}
	return session
}

func TestConsentAuthorizationAndRelay(t *testing.T) {
	ctx := context.Background()
	s, now := newTestService(t)
	session := invite(t, s)
	if !codePattern.MatchString(session.Code) || session.Expires != now.Add(InvitationTTL).Unix() {
		t.Fatalf("invalid invitation: %+v", session)
	}
	for _, actor := range []string{alice, eve} {
		if _, err := s.Accept(ctx, actor, session.Code); !errors.Is(err, ErrSession) {
			t.Fatalf("unauthorized acceptance: %v", err)
		}
	}
	if _, err := s.Reply(ctx, alice, session.Code, "too soon"); !errors.Is(err, ErrPending) {
		t.Fatalf("pending relay allowed: %v", err)
	}
	if _, err := s.Reply(ctx, eve, session.Code, "intruder"); !errors.Is(err, ErrSession) {
		t.Fatalf("outsider relay: %v", err)
	}
	if err := s.Stop(ctx, eve, session.Code); !errors.Is(err, ErrSession) {
		t.Fatalf("outsider stopped session: %v", err)
	}
	relay, err := s.Accept(ctx, bob, strings.ToUpper(session.Code))
	if err != nil || relay.To != bob || relay.Notify != alice || relay.Alias != "Bintang" || relay.Text != "Halo rahasia" {
		t.Fatalf("accept: %+v %v", relay, err)
	}
	if _, err := s.Accept(ctx, bob, session.Code); !errors.Is(err, ErrSession) {
		t.Fatalf("duplicate acceptance: %v", err)
	}
	relay, err = s.Reply(ctx, bob, session.Code, "Siapa?")
	if err != nil || relay.To != alice || relay.Alias != "Penerima" {
		t.Fatalf("recipient relay: %+v %v", relay, err)
	}
	if _, err := s.Reply(ctx, bob, session.Code, "spam"); !errors.Is(err, ErrCooldown) {
		t.Fatalf("cooldown bypassed: %v", err)
	}
	*now = now.Add(5 * time.Second)
	relay, err = s.Reply(ctx, alice, session.Code, "Temanmu")
	if err != nil || relay.To != bob || relay.Alias != "Bintang" {
		t.Fatalf("sender relay: %+v %v", relay, err)
	}
	var stored Session
	if err := s.db.Get(&stored, `SELECT * FROM menfess_sessions`); err != nil {
		t.Fatal(err)
	}
	if stored.Initial != "" || stored.Expires != now.Add(SessionTTL-5*time.Second).Unix() {
		t.Fatalf("bad active data: %+v", stored)
	}
	if err := s.Stop(ctx, bob, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Reply(ctx, alice, session.Code, "closed"); !errors.Is(err, ErrSession) {
		t.Fatalf("closed relay: %v", err)
	}
	if err := s.db.Get(&stored, `SELECT * FROM menfess_sessions`); err != nil {
		t.Fatal(err)
	}
	if stored.Initial != "" || stored.Alias != "" {
		t.Fatal("closed content not cleared")
	}
}

func TestBlockingPersistsAcrossRestart(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestService(t)
	session := invite(t, s)
	if err := s.Block(ctx, bob, true); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(ctx, s.db)
	if err != nil {
		t.Fatal(err)
	}
	reopened.now = s.now
	if _, err := reopened.Accept(ctx, bob, session.Code); !errors.Is(err, ErrSession) {
		t.Fatalf("blocked session survived: %v", err)
	}
	if _, err := reopened.Invite(ctx, eve, bob, "Anon", "Hello"); !errors.Is(err, ErrBlocked) {
		t.Fatalf("block not persisted: %v", err)
	}
	if err := reopened.Block(ctx, bob, false); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.Accept(ctx, bob, session.Code); !errors.Is(err, ErrSession) {
		t.Fatal("unblock resurrected old session")
	}
	if _, err := reopened.Invite(ctx, eve, bob, "Anon", "Hello"); err != nil {
		t.Fatal(err)
	}
}

func TestExpiryAndRetention(t *testing.T) {
	ctx := context.Background()
	for _, active := range []bool{false, true} {
		t.Run(fmt.Sprint(active), func(t *testing.T) {
			s, now := newTestService(t)
			session := invite(t, s)
			ttl := InvitationTTL
			if active {
				if _, err := s.Accept(ctx, bob, session.Code); err != nil {
					t.Fatal(err)
				}
				ttl = SessionTTL
			}
			*now = now.Add(ttl)
			if _, err := s.Reply(ctx, alice, session.Code, "expired"); !errors.Is(err, ErrSession) {
				t.Fatalf("expired relay: %v", err)
			}
			var stored Session
			if err := s.db.Get(&stored, `SELECT * FROM menfess_sessions`); err != nil {
				t.Fatal(err)
			}
			if stored.State != "expired" || stored.Initial != "" || stored.Alias != "" {
				t.Fatalf("expiry was not committed: %+v", stored)
			}
			*now = now.Add(49 * time.Hour)
			if err := s.Prune(ctx); err != nil {
				t.Fatal(err)
			}
			var count int
			if err := s.db.Get(&count, `SELECT COUNT(*) FROM menfess_sessions`); err != nil || count != 0 {
				t.Fatalf("retention: %d %v", count, err)
			}
		})
	}
}

func TestInvitationLimits(t *testing.T) {
	ctx := context.Background()
	t.Run("sender", func(t *testing.T) {
		s, now := newTestService(t)
		for i := 0; i < 3; i++ {
			session := invite(t, s)
			if err := s.Stop(ctx, bob, session.Code); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := s.Invite(ctx, alice, bob, "Anon", "Spam"); !errors.Is(err, ErrLimit) {
			t.Fatalf("sender limit: %v", err)
		}
		*now = now.Add(time.Hour)
		invite(t, s)
	})
	t.Run("recipient", func(t *testing.T) {
		s, _ := newTestService(t)
		for i := 0; i < 5; i++ {
			sender := fmt.Sprintf("62810000000%d@s.whatsapp.net", i)
			session, err := s.Invite(ctx, sender, bob, "Anon", "Hello")
			if err != nil {
				t.Fatal(err)
			}
			if err := s.Stop(ctx, bob, session.Code); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := s.Invite(ctx, alice, bob, "Anon", "Spam"); !errors.Is(err, ErrLimit) {
			t.Fatalf("recipient limit: %v", err)
		}
	})
}

func TestConcurrentInvitationsAndAcceptance(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestService(t)
	var sequence int
	var databaseName, databaseFile string
	if err := s.db.QueryRow(`PRAGMA database_list`).Scan(&sequence, &databaseName, &databaseFile); err != nil {
		t.Fatal(err)
	}
	independentDB, err := sqlx.Open("sqlite", databaseFile+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	independentDB.SetMaxOpenConns(1)
	t.Cleanup(func() { independentDB.Close() })
	second, err := New(ctx, independentDB)
	if err != nil {
		t.Fatal(err)
	}
	second.now = s.now
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i, service := range []*Service{s, second} {
		wg.Add(1)
		go func(i int, service *Service) {
			defer wg.Done()
			sender := alice
			if i == 1 {
				sender = eve
			}
			_, err := service.Invite(ctx, sender, bob, "Anon", "Hello")
			results <- err
		}(i, service)
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else if !errors.Is(err, ErrBusy) {
			t.Fatal(err)
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent invitations=%d", successes)
	}
	var session Session
	if err := s.db.Get(&session, `SELECT * FROM menfess_sessions`); err != nil {
		t.Fatal(err)
	}
	results = make(chan error, 2)
	for _, service := range []*Service{s, second} {
		wg.Add(1)
		go func(service *Service) {
			defer wg.Done()
			_, err := service.Accept(ctx, bob, session.Code)
			results <- err
		}(service)
	}
	wg.Wait()
	close(results)
	successes = 0
	for err := range results {
		if err == nil {
			successes++
		} else if !errors.Is(err, ErrSession) {
			t.Fatal(err)
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent acceptances=%d", successes)
	}
}

func TestInputValidation(t *testing.T) {
	s, _ := newTestService(t)
	for _, text := range []string{"", strings.Repeat("あ", 1001), "hello\x00world"} {
		if _, err := s.Invite(context.Background(), alice, bob, "Anon", text); err == nil {
			t.Errorf("accepted bad text %q", text)
		}
	}
	for _, alias := range []string{"", strings.Repeat("a", 41), "fake\nname"} {
		if _, err := s.Invite(context.Background(), alice, bob, alias, "Hello"); err == nil {
			t.Errorf("accepted bad alias %q", alias)
		}
	}
	if _, err := s.Invite(context.Background(), alice, alice, "Anon", "Hello"); err == nil {
		t.Fatal("self invite accepted")
	}
	if _, err := s.Invite(context.Background(), alice, "123@lid", "Anon", "Hello"); err == nil {
		t.Fatal("unstable LID accepted")
	}
}

func TestAcceptedSessionSurvivesRestart(t *testing.T) {
	ctx := context.Background()
	s, now := newTestService(t)
	session := invite(t, s)
	if _, err := s.Accept(ctx, bob, session.Code); err != nil {
		t.Fatal(err)
	}
	restarted, err := New(ctx, s.db)
	if err != nil {
		t.Fatal(err)
	}
	restarted.now = s.now
	*now = now.Add(5 * time.Second)
	relay, err := restarted.Reply(ctx, alice, session.Code, "After restart")
	if err != nil || relay.To != bob || relay.Text != "After restart" {
		t.Fatalf("persistent relay: %+v %v", relay, err)
	}
}
