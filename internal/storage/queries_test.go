package storage

import (
	"errors"
	"hockey_bot/internal/domain"
	"reflect"
	"sort"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestTeamQueriesAndAdminDeduplication(t *testing.T) {
	st := newTrainingTestStorage(t)
	chat := int64(-10)
	teams := []domain.Team{{ID: "a", Name: "A", Admins: []int64{42, 42}}, {ID: "b", Name: "B", ChatID: &chat, Admins: []int64{42, 7}}, {ID: "c", Name: "C", Admins: []int64{7}}}
	for i := range teams {
		if err := st.CreateTeam(&teams[i]); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.SaveTeam(&teams[0]); err != nil {
		t.Fatal(err)
	}
	a, err := st.GetTeamByName("A")
	if err != nil || !reflect.DeepEqual(a.Admins, []int64{42}) {
		t.Fatalf("admins not deduplicated: %+v %v", a, err)
	}
	assertIDs := func(got []domain.Team, err error, want ...string) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		ids := make([]string, 0, len(got))
		for _, v := range got {
			ids = append(ids, v.ID)
		}
		sort.Strings(ids)
		if !reflect.DeepEqual(ids, want) {
			t.Fatalf("teams = %v, want %v", ids, want)
		}
	}
	got, err := st.GetTeamsByAdmin(42)
	assertIDs(got, err, "a", "b")
	got, err = st.GetTeamsWithoutChatByAdmin(42)
	assertIDs(got, err, "a")
	got, err = st.GetTeamsWithChat()
	assertIDs(got, err, "b")
	b, err := st.GetTeamByChatID(chat)
	if err != nil || b.ID != "b" {
		t.Fatalf("wrong chat team: %+v %v", b, err)
	}
	for _, tc := range []struct {
		team string
		user int64
		want bool
	}{{"a", 42, true}, {"a", 7, false}, {"absent", 42, false}} {
		got, err := st.IsAdmin(tc.team, tc.user)
		if err != nil || got != tc.want {
			t.Fatalf("admin %v: %t %v", tc, got, err)
		}
	}
	for _, get := range []func(string) (*domain.Team, error){st.GetTeamByID, st.GetTeamByName} {
		got, err := get("absent")
		if err != nil || got != nil {
			t.Fatalf("missing team: %+v %v", got, err)
		}
	}
	if _, err := st.GetTeamByChatID(999); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("missing chat: %v", err)
	}
	if err := st.UpdateTeamChatID("a", -20); err != nil {
		t.Fatal(err)
	}
	a, err = st.GetTeamByID("a")
	if err != nil || a.ChatID == nil || *a.ChatID != -20 {
		t.Fatal("chat ID not persisted")
	}
}

func TestPlayerQueriesKeepOwnerAndFriendsWithinTeam(t *testing.T) {
	st := newTrainingTestStorage(t)
	base := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	players := []domain.Player{
		{ID: "owner", TeamID: "a", TelegramID: 42, FullName: "Иванов", CreatedAt: base},
		{ID: "friend", TeamID: "a", TelegramID: 42, FullName: "Петров", CreatedAt: base.Add(time.Second)},
		{ID: "other-team", TeamID: "b", TelegramID: 42, FullName: "Иванов", CreatedAt: base.Add(-time.Second)},
		{ID: "other-user", TeamID: "a", TelegramID: 77, FullName: "Сидоров", CreatedAt: base.Add(2 * time.Second)},
	}
	for i := range players {
		if err := st.CreatePlayer(&players[i]); err != nil {
			t.Fatal(err)
		}
	}
	owner, err := st.GetPlayer(42, "a")
	if err != nil || owner.ID != "owner" {
		t.Fatalf("wrong owner: %+v %v", owner, err)
	}
	owned, err := st.GetPlayersByTelegramID("a", 42)
	if err != nil || len(owned) != 2 || owned[0].ID != "owner" || owned[1].ID != "friend" {
		t.Fatalf("wrong owned players: %v %v", owned, err)
	}
	all, err := st.GetPlayersByTeamID("a")
	if err != nil || len(all) != 3 {
		t.Fatalf("team leaked players: %v %v", all, err)
	}
	exists, err := st.PlayerExists("a", 42)
	if err != nil || !exists {
		t.Fatal("existing player not found")
	}
	exists, err = st.PlayerExists("b", 77)
	if err != nil || exists {
		t.Fatal("foreign player found")
	}
	byID, err := st.GetPlayerByID("friend")
	if err != nil || byID.FullName != "Петров" {
		t.Fatal("wrong friend lookup")
	}
	missing, err := st.GetPlayerByID("absent")
	if err != nil || missing != nil {
		t.Fatal("missing player lookup")
	}
	missing, err = st.GetPlayer(999, "a")
	if err != nil || missing != nil {
		t.Fatal("missing owner lookup")
	}
}

func TestTrainingQueriesFilterTeamDateTypeAndLimit(t *testing.T) {
	st := newTrainingTestStorage(t)
	p := domain.Player{ID: "owner", TeamID: "a", TelegramID: 42}
	if err := st.CreatePlayer(&p); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	events := []domain.Training{
		{ID: "past", TeamID: "a", Type: domain.TrainingGaming, Date: now.Add(-time.Hour), Venue: "Старая арена"},
		{ID: "near", TeamID: "a", Type: domain.TrainingGaming, Date: now.Add(time.Hour), Venue: "Арена"},
		{ID: "far", TeamID: "a", Type: domain.TrainingTechnical, Date: now.Add(2 * time.Hour), Venue: "Арена"},
		{ID: "game", TeamID: "a", Type: domain.TrainingMatch, Date: now.Add(time.Hour)},
		{ID: "foreign", TeamID: "b", Type: domain.TrainingGaming, Date: now.Add(time.Hour), Venue: "Чужая арена"},
	}
	for i := range events {
		if err := st.CreateTraining(&events[i]); err != nil {
			t.Fatal(err)
		}
	}
	got, err := st.GetUpcomingTrainingsByTelegramID(42, 1)
	if err != nil || len(got) != 1 || got[0].ID != "near" {
		t.Fatalf("limit or filters broken: %v %v", got, err)
	}
	got, err = st.GetUpcomingTrainingsByTelegramID(42, 0)
	if err != nil || len(got) != 2 || got[1].ID != "far" {
		t.Fatalf("default limit or ordering broken: %v %v", got, err)
	}
	got, err = st.GetUpcomingGames("a", 0)
	if err != nil || len(got) != 1 || got[0].ID != "game" {
		t.Fatalf("game filter broken: %v %v", got, err)
	}
	got, err = st.GetUpcomingTrainingsByTelegramID(999, 10)
	if err != nil || len(got) != 0 {
		t.Fatal("unknown user received trainings")
	}
	venues := st.GetRecentVenues("a")
	sort.Strings(venues)
	if !reflect.DeepEqual(venues, []string{"Арена", "Старая арена"}) {
		t.Fatalf("venue dedup/team filter broken: %v", venues)
	}
	if err := st.UpdateTeamMessageID("near", 17); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateApprovedMessageID("near", 18); err != nil {
		t.Fatal(err)
	}
	event, err := st.GetTrainingByID("near")
	if err != nil || event.TeamMessageID != 17 || event.ApprovedMessageID != 18 {
		t.Fatal("message IDs not saved")
	}
	event, err = st.GetTrainingByID("absent")
	if err != nil || event != nil {
		t.Fatal("missing training lookup")
	}
}

func TestApprovalTransactionRollsBackOnCapacityFailure(t *testing.T) {
	st := newTrainingTestStorage(t)
	players := []domain.Player{{ID: "a"}, {ID: "b"}}
	if err := st.db.Create(&players).Error; err != nil {
		t.Fatal(err)
	}
	tr := domain.Training{ID: "training", Players: players}
	if err := st.CreateTraining(&tr); err != nil {
		t.Fatal(err)
	}
	if err := st.SetApprovedPlayers(tr.ID, []string{"a"}); err != nil {
		t.Fatal(err)
	}
	if err := st.db.Exec(`CREATE TRIGGER reject_capacity BEFORE UPDATE OF capacity ON trainings BEGIN SELECT RAISE(ABORT, 'capacity failure'); END;`).Error; err != nil {
		t.Fatal(err)
	}
	if err := st.SetApprovedPlayers(tr.ID, []string{"b"}); err == nil {
		t.Fatal("injected failure ignored")
	}
	got, err := st.GetApprovedPlayerIDs(tr.ID)
	if err != nil || !reflect.DeepEqual(got, []string{"a"}) {
		t.Fatalf("failed transaction changed approvals: %v %v", got, err)
	}
}

func TestCreatingTrainingRollsBackIfEnrollmentFails(t *testing.T) {
	st := newTrainingTestStorage(t)
	if err := st.db.Exec(`CREATE TRIGGER reject_signup BEFORE INSERT ON training_players BEGIN SELECT RAISE(ABORT, 'signup failure'); END;`).Error; err != nil {
		t.Fatal(err)
	}
	err := st.CreateTraining(&domain.Training{ID: "failed", Players: []domain.Player{{ID: "p"}}})
	if err == nil {
		t.Fatal("injected failure ignored")
	}
	got, err := st.GetTrainingByID("failed")
	if err != nil || got != nil {
		t.Fatalf("half-created training left behind: %+v %v", got, err)
	}
}
