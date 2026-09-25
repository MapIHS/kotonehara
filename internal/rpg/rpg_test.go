package rpg

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

type repeatingByte byte

func (v repeatingByte) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(v)
	}
	return len(p), nil
}
func openTest(t *testing.T, path string) (*sqlx.DB, *Service) {
	t.Helper()
	db, err := sqlx.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	t.Cleanup(func() { _ = db.Close() })
	s, err := New(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	s.random = repeatingByte(0x10)
	return db, s
}
func fixture(t *testing.T) (*sqlx.DB, *Service, Profile) {
	t.Helper()
	db, s := openTest(t, filepath.Join(t.TempDir(), "game.db"))
	p, err := s.EnsurePlayer(context.Background(), []string{"628111@s.whatsapp.net", "111@lid"}, "Ihsan")
	if err != nil {
		t.Fatal(err)
	}
	return db, s, p
}
func requireCode(t *testing.T, err error, code string) {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) || e.Code != code {
		t.Fatalf("want %s, got %v", code, err)
	}
}
func patchProfile(t *testing.T, db *sqlx.DB, p Profile) {
	t.Helper()
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE rpg_players SET state=?,revision=? WHERE id=?`, string(raw), p.Revision, p.ID); err != nil {
		t.Fatal(err)
	}
}

func TestProfileIdentityAndMigration(t *testing.T) {
	db, s, p := fixture(t)
	ctx := context.Background()
	for _, aliases := range [][]string{{"111@lid"}, {"628111@s.whatsapp.net"}, {"111@lid", "628111@s.whatsapp.net"}} {
		other, err := s.EnsurePlayer(ctx, aliases, "Changed")
		if err != nil {
			t.Fatal(err)
		}
		if other.ID != p.ID || other.Shards != 1600 {
			t.Fatal("identity created another wallet")
		}
	}
	var welcome int
	if err := db.Get(&welcome, `SELECT COUNT(*) FROM rpg_ledger WHERE event_key='welcome'`); err != nil {
		t.Fatal(err)
	}
	if welcome != 1 {
		t.Fatal(welcome)
	}
	if _, err := New(ctx, db); err != nil {
		t.Fatal(err)
	}
	out, err := s.Snapshot(ctx, p.ID)
	if err != nil || !reflect.DeepEqual(out.Profile, p) {
		t.Fatalf("migration changed existing data: %v", err)
	}
	other, err := s.EnsurePlayer(ctx, []string{"222@lid"}, "Other")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.EnsurePlayer(ctx, []string{"222@lid", "111@lid"}, "Conflict")
	requireCode(t, err, "identity_conflict")
	if other.ID == p.ID {
		t.Fatal("accounts merged")
	}
}
func TestTicketsSessionsAndExpiry(t *testing.T) {
	_, s, p := fixture(t)
	ctx := context.Background()
	now := time.Now()
	s.now = func() time.Time { return now }
	first, err := s.IssueTicket(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := s.IssueTicket(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ExchangeTicket(ctx, first)
	requireCode(t, err, "ticket_invalid")
	var wg sync.WaitGroup
	results := make(chan string, 2)
	for range 2 {
		wg.Go(func() {
			session, err := s.ExchangeTicket(ctx, ticket)
			if err == nil {
				results <- session
			} else {
				var e *Error
				if !errors.As(err, &e) || e.Code != "ticket_invalid" {
					t.Errorf("exchange: %v", err)
				}
			}
		})
	}
	wg.Wait()
	close(results)
	sessions := []string{}
	for session := range results {
		sessions = append(sessions, session)
	}
	if len(sessions) != 1 {
		t.Fatal("ticket consumed more than once")
	}
	player, err := s.SessionPlayer(ctx, sessions[0])
	if err != nil || player != p.ID {
		t.Fatal(player, err)
	}
	ticket, err = s.IssueTicket(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute)
	_, err = s.ExchangeTicket(ctx, ticket)
	requireCode(t, err, "ticket_invalid")
	now = now.Add(7 * 24 * time.Hour)
	_, err = s.SessionPlayer(ctx, sessions[0])
	requireCode(t, err, "session_expired")
}
func TestGachaPityRetryAndRace(t *testing.T) {
	db, s, p := fixture(t)
	ctx := context.Background()
	p.Pity4 = 9
	patchProfile(t, db, p)
	out, err := s.Summon(ctx, p.ID, "guarantee-four", BannerID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if out.Results[0].Character.Rarity != 4 || out.Profile.Pity4 != 0 {
		t.Fatal("missing 4-star pity")
	}
	again, err := s.Summon(ctx, p.ID, "guarantee-four", BannerID, 1)
	if err != nil || !reflect.DeepEqual(out, again) {
		t.Fatalf("replay changed outcome: %v", err)
	}
	_, err = s.Summon(ctx, p.ID, "guarantee-four", BannerID, 10)
	requireCode(t, err, "request_reused")
	p = out.Profile
	p.Pity5 = 79
	p.Pity4 = 9
	p.Guarantee = true
	patchProfile(t, db, p)
	out, err = s.Summon(ctx, p.ID, "guarantee-five", BannerID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if out.Results[0].Character.ID != s.catalog.Featured || out.Profile.Pity5 != 0 || out.Profile.Pity4 != 0 || out.Profile.Guarantee {
		t.Fatal("5-star/featured guarantee failed")
	}
	p = out.Profile
	p.Pity5 = 79
	p.Guarantee = false
	patchProfile(t, db, p)
	s.random = repeatingByte(0x11)
	out, err = s.Summon(ctx, p.ID, "lose-featured", BannerID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if out.Results[0].Character.ID == s.catalog.Featured || !out.Profile.Guarantee {
		t.Fatal("missing next featured guarantee")
	}
	p = out.Profile
	p.Shards = 1600
	patchProfile(t, db, p)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := range 2 {
		wg.Go(func() { _, err := s.Summon(ctx, p.ID, fmt.Sprintf("race-draw-%d", i), BannerID, 10); results <- err })
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else {
			requireCode(t, err, "funds")
		}
	}
	if success != 1 {
		t.Fatal("concurrent draws overspent wallet")
	}
}
func TestGachaRollsBackEntireBatch(t *testing.T) {
	db, s, p := fixture(t)
	ctx := context.Background()
	s.random = io.LimitReader(repeatingByte(0x10), 7)
	_, err := s.Summon(ctx, p.ID, "failed-batch", BannerID, 10)
	if err == nil {
		t.Fatal("expected RNG read failure")
	}
	out, err := s.Snapshot(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(out.Profile, p) {
		t.Fatal("partial wallet or collection update")
	}
	var rows int
	if err = db.Get(&rows, `SELECT COUNT(*) FROM rpg_gacha_history`); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatal("partial history")
	}
	s.random = repeatingByte(0x10)
	out, err = s.Summon(ctx, p.ID, "failed-batch", BannerID, 10)
	if err != nil || len(out.Results) != 10 || out.Profile.Shards != 0 {
		t.Fatal("retry after rollback", err)
	}
}
func winBattle(t *testing.T, s *Service, p string, b *Battle, prefix string) *Battle {
	t.Helper()
	for turn := 0; turn < 100; turn++ {
		if b.Done {
			if b.Result != "win" {
				t.Fatalf("battle lost at stage %d", b.Stage)
			}
			return b
		}
		h := b.Heroes[b.Active]
		action := "attack"
		if h.Energy >= 5 {
			action = "ultimate"
		} else if h.Energy >= 2 && h.Role != "Guardian" {
			action = "skill"
		}
		out, err := s.Act(context.Background(), p, b.ID, Action{RequestID: fmt.Sprintf("%s-%03d", prefix, turn), Revision: b.Revision, Actor: b.Active, Target: b.Target, Action: action})
		if err != nil {
			t.Fatal(err)
		}
		b = out.Battle
	}
	t.Fatal("battle did not end")
	return nil
}
func TestBattlePersistenceRewardsAndRevisions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "persist.db")
	db, s := openTest(t, path)
	ctx := context.Background()
	p, err := s.EnsurePlayer(ctx, []string{"333@lid"}, "Player")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.StartBattle(ctx, p.ID, "locked-stage", 1)
	requireCode(t, err, "stage_locked")
	out, err := s.StartBattle(ctx, p.ID, "first-battle", 0)
	if err != nil {
		t.Fatal(err)
	}
	b := out.Battle
	_, err = s.StartBattle(ctx, p.ID, "second-battle", 0)
	requireCode(t, err, "battle_active")
	_, err = s.SetParty(ctx, p.ID, PartyRequest{RequestID: "change-party", Revision: out.Profile.Revision, Party: p.Party})
	requireCode(t, err, "battle_active")
	a := Action{RequestID: "first-action", Revision: b.Revision, Actor: 0, Action: "attack", Target: 1}
	out, err = s.Act(ctx, p.ID, b.ID, a)
	if err != nil {
		t.Fatal(err)
	}
	if out.Battle.Enemies[1].HP >= b.Enemies[1].HP || out.Battle.Enemies[0].HP != b.Enemies[0].HP {
		t.Fatal("target handling")
	}
	replay, err := s.Act(ctx, p.ID, b.ID, a)
	if err != nil || !reflect.DeepEqual(out, replay) {
		t.Fatalf("action replay: %v", err)
	}
	a.RequestID = "stale-action"
	_, err = s.Act(ctx, p.ID, b.ID, a)
	requireCode(t, err, "stale_revision")
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	_, s = openTest(t, path)
	resumed, err := s.Snapshot(ctx, p.ID)
	if err != nil || !reflect.DeepEqual(out, resumed) {
		t.Fatalf("restart lost battle: %v", err)
	}
	b = winBattle(t, s, p.ID, resumed.Battle, "story")
	if !b.FirstClear || b.RewardShards != 20 {
		t.Fatal("first reward")
	}
	after, _ := s.Snapshot(ctx, p.ID)
	if after.Profile.Shards != 1620 || after.Profile.Unlocked != 1 {
		t.Fatal(after.Profile)
	}
	out, err = s.StartBattle(ctx, p.ID, "replay-battle", 0)
	if err != nil {
		t.Fatal(err)
	}
	b = winBattle(t, s, p.ID, out.Battle, "practice")
	if b.FirstClear || b.RewardShards != 0 {
		t.Fatal("duplicate first reward")
	}
	after, _ = s.Snapshot(ctx, p.ID)
	if after.Profile.Shards != 1620 {
		t.Fatal("reward duplicated")
	}
}
func TestBattleOwnershipConcurrentActionAndRetreat(t *testing.T) {
	_, s, p := fixture(t)
	ctx := context.Background()
	other, err := s.EnsurePlayer(ctx, []string{"999@lid"}, "Other")
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.StartBattle(ctx, p.ID, "own-battle", 0)
	if err != nil {
		t.Fatal(err)
	}
	b := out.Battle
	_, err = s.Act(ctx, other.ID, b.ID, Action{RequestID: "attack-other", Actor: 0, Target: 0, Action: "attack"})
	requireCode(t, err, "battle_missing")
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := range 2 {
		wg.Go(func() {
			_, err := s.Act(ctx, p.ID, b.ID, Action{RequestID: fmt.Sprintf("concurrent-%d", i), Revision: b.Revision, Actor: 0, Target: 0, Action: "guard"})
			results <- err
		})
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else {
			requireCode(t, err, "stale_revision")
		}
	}
	if success != 1 {
		t.Fatal("multiple actions on same revision")
	}
	out, err = s.Snapshot(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	out, err = s.Retreat(ctx, p.ID, b.ID, "retreat-once", out.Battle.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if !out.Battle.Done || out.Profile.Shards != 1600 || len(out.Profile.Cleared) != 0 {
		t.Fatal("retreat rewarded")
	}
	_, err = s.SetParty(ctx, p.ID, PartyRequest{RequestID: "invalid-party", Revision: out.Profile.Revision, Party: []string{"char_001", "char_001", "char_001", "char_001"}})
	requireCode(t, err, "party")
	party := []string{"char_023", "char_004", "char_002", "char_001"}
	out, err = s.SetParty(ctx, p.ID, PartyRequest{RequestID: "valid-party", Revision: out.Profile.Revision, Party: party})
	if err != nil || !reflect.DeepEqual(out.Profile.Party, party) {
		t.Fatal("party update", err)
	}
}
func TestAllLevelsCatalogAndBossScaling(t *testing.T) {
	db, s, p := fixture(t)
	if len(s.catalog.Stages) != 999 {
		t.Fatal("incomplete levels")
	}
	for _, level := range []int{1, 99, 100, 499, 999} {
		p.Unlocked = level - 1
		patchProfile(t, db, p)
		out, err := s.StartBattle(context.Background(), p.ID, fmt.Sprintf("level-start-%d", level), level-1)
		if err != nil {
			t.Fatal(err)
		}
		if out.Battle.Stage != level-1 || out.Battle.Heroes[0].HP <= 0 || out.Battle.Enemies[0].HP <= 0 {
			t.Fatal(level)
		}
		winBattle(t, s, p.ID, out.Battle, fmt.Sprintf("level-fight-%d", level))
		out, err = s.Snapshot(context.Background(), p.ID)
		if err != nil {
			t.Fatal(err)
		}
		p = out.Profile
	}
}

func TestSoftPityBoundaries(t *testing.T) {
	_, s, _ := fixture(t)
	for _, test := range []struct {
		name         string
		since5, roll int
		want5        bool
	}{
		{"before-soft", 59, 699, false},
		{"first-soft", 60, 699, true},
		{"first-soft-exact-boundary", 60, 700, false},
		{"last-soft", 78, 9700, false},
		{"hard-guarantee", 79, 9999, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			s.random = io.MultiReader(bytes.NewReader([]byte{byte(test.roll >> 8), byte(test.roll)}), repeatingByte(0x10))
			p := Profile{Pity5: test.since5, Collection: map[string]int{}}
			pull, err := s.pull(&p)
			if err != nil {
				t.Fatal(err)
			}
			if (pull.Character.Rarity == 5) != test.want5 {
				t.Fatalf("roll %d at pity %d produced rarity %d", test.roll, test.since5, pull.Character.Rarity)
			}
		})
	}
}
func TestStorageFailureRollsBackWalletHistoryAndLedger(t *testing.T) {
	db, s, p := fixture(t)
	_, err := db.Exec(`CREATE TRIGGER reject_test_receipt BEFORE INSERT ON rpg_requests WHEN NEW.request_id='storage-failure' BEGIN SELECT RAISE(ABORT,'simulated storage failure'); END`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Summon(context.Background(), p.ID, "storage-failure", BannerID, 10)
	if err == nil {
		t.Fatal("expected receipt write failure")
	}
	after, err := s.Snapshot(context.Background(), p.ID)
	if err != nil || !reflect.DeepEqual(after.Profile, p) {
		t.Fatal("wallet/pity/collection not rolled back", err)
	}
	for _, table := range []string{"rpg_gacha_history", "rpg_requests"} {
		var n int
		if err = db.Get(&n, "SELECT COUNT(*) FROM "+table); err != nil || n != 0 {
			t.Fatal("partial write in", table, err)
		}
	}
	var n int
	if err = db.Get(&n, `SELECT COUNT(*) FROM rpg_ledger WHERE event_key<>'welcome'`); err != nil || n != 0 {
		t.Fatal("partial ledger", err)
	}
	if _, err = db.Exec(`DROP TRIGGER reject_test_receipt`); err != nil {
		t.Fatal(err)
	}
	out, err := s.Summon(context.Background(), p.ID, "storage-failure", BannerID, 10)
	if err != nil || len(out.Results) != 10 || out.Profile.Shards != 0 {
		t.Fatal("retry after failure", err)
	}
}
