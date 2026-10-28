package scenario

import (
	"encoding/json"
	"errors"
	"fmt"
	"hockey_bot/internal/bot/user_state"
	"hockey_bot/internal/domain"
	"hockey_bot/internal/pendingreply"
	"hockey_bot/internal/storage"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func scenarioFixture(t *testing.T) (storage.Storage, domain.Team, domain.Player, domain.Training) {
	t.Helper()
	st := storage.NewGORM(filepath.Join(t.TempDir(), "bot.db"))
	t.Cleanup(st.Close)
	chatID := int64(-100)
	team := domain.Team{ID: "team", Name: "Команда", Admins: []int64{1}, ChatID: &chatID}
	player := domain.Player{ID: "player", TeamID: team.ID, TelegramID: 42, FullName: "Игрок"}
	training := domain.Training{ID: "training", TeamID: team.ID, Team: team, Date: time.Now().Add(time.Hour), Players: []domain.Player{player}}
	if err := st.CreateTeam(&team); err != nil {
		t.Fatal(err)
	}
	if err := st.CreatePlayer(&player); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateTraining(&training); err != nil {
		t.Fatal(err)
	}
	return st, team, player, training
}

func runStep(t *testing.T, sc Scenario, index int, st storage.Storage, chatID, userID int64, data Data) (*Action, error) {
	t.Helper()
	step, err := sc.GetStep(index)
	if err != nil {
		t.Fatal(err)
	}
	return step(st, chatID, userID, 10, data)
}

func TestCancelReasonSurvivesPrivateAndGroupCancellation(t *testing.T) {
	for _, chatID := range []int64{42, -100} {
		t.Run(fmt.Sprint(chatID), func(t *testing.T) {
			st, team, _, training := scenarioFixture(t)
			user_state.SetUserState(42, chatID, "nogo_tr:0")
			action, err := runStep(t, NewCancelJoinTraining(), 0, st, chatID, 42, Data{"id": training.ID})
			if err != nil || action.PendingReplyContext == nil {
				t.Fatalf("cancel: %+v, %v", action, err)
			}
			pendingreply.Register(nil, st, action.PendingReplyContext)
			t.Cleanup(func() { pendingreply.Resolve(42, training.ID) })
			value := user_state.GetUserState(42, 42)
			if !strings.HasPrefix(value, "cancel_reason:0:") {
				t.Fatalf("reason state was cleared: %q", value)
			}
			if chatID < 0 && user_state.GetUserState(42, chatID) != "" {
				t.Fatal("group cancel state was not cleared")
			}
			var data Data
			if err := json.Unmarshal([]byte(strings.SplitN(value, ":", 3)[2]), &data); err != nil {
				t.Fatal(err)
			}
			data["message"] = "Заболел"
			action, err = runStep(t, NewCancelReason(), 0, st, 42, 42, data)
			if err != nil {
				t.Fatal(err)
			}
			notified := false
			for _, reply := range action.Reply {
				if reply.ChatID == team.Admins[0] && strings.Contains(reply.Text, "Заболел") {
					notified = true
				}
			}
			if !notified {
				t.Fatal("coach did not receive the reason")
			}
			if user_state.GetUserState(42, 42) != "" {
				t.Fatal("reason state was not cleared after reply")
			}
		})
	}
}

func TestAdministrativeStepsRejectNonAdminAndRevokedAccess(t *testing.T) {
	st, team, player, training := scenarioFixture(t)
	training.Type = domain.TrainingMatch
	if err := st.SaveTraining(&training); err != nil {
		t.Fatal(err)
	}
	key := sessionKey{42, 42}
	tests := []struct {
		name  string
		sc    Scenario
		index int
		data  Data
	}{
		{"manage", NewManageTeam(), 1, Data{"team_id": team.ID}},
		{"players", NewPlayersList(), 0, Data{"id": team.ID}},
		{"bind chat", NewAddBot(), 1, Data{"id": team.ID}},
		{"set admin menu", NewSetAdmin(), 0, Data{"id": team.ID}},
		{"delete admin menu", NewDelAdmin(), 0, Data{"id": team.ID}},
		{"set admin final", NewSetAdmin(), 1, Data{"id": player.ID}},
		{"delete admin final", NewDelAdmin(), 1, Data{"id": player.ID}},
		{"schedule training", NewScheduleTraining(), 0, Data{"team_id": team.ID}},
		{"schedule game", NewScheduleGame(), 0, Data{"team_id": team.ID}},
		{"create training", NewScheduleTraining(), 13, Data{}},
		{"create game", NewScheduleGame(), 9, Data{}},
		{"choose lineup", NewApproveLineup(), 0, Data{"ti": team.ID}},
		{"change lineup", NewApproveLineup(), 1, Data{"g": training.ID}},
		{"confirm lineup", NewApproveLineup(), 2, Data{"g": training.ID}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Подготовленные ранее черновики не дают прав после разжалования.
			localTrainingID[key] = team.ID
			globalUserTraining[key] = training
			approveLineupStateByUser[key] = &approveLineupState{TeamID: team.ID, GameID: training.ID, ApprovedIDs: map[string]bool{player.ID: true}}
			chatID := int64(42)
			if tc.name == "bind chat" {
				chatID = -100
			}
			_, err := runStep(t, tc.sc, tc.index, st, chatID, 42, tc.data)
			if !errors.Is(err, ErrForbidden) {
				t.Fatalf("expected forbidden, got %v", err)
			}
		})
	}
	got, err := st.GetTeamByID(team.ID)
	if err != nil || len(got.Admins) != 1 || got.Admins[0] != 1 {
		t.Fatalf("team changed: %+v %v", got, err)
	}
}

func TestAdminCanBindAndCannotAssignForeignPlayer(t *testing.T) {
	st, team, player, _ := scenarioFixture(t)
	if _, err := runStep(t, NewAddBot(), 1, st, -200, 1, Data{"id": team.ID}); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetTeamByID(team.ID)
	if err != nil || got.ChatID == nil || *got.ChatID != -200 {
		t.Fatalf("chat was not bound: %+v %v", got, err)
	}
	localTrainingID[sessionKey{1, 1}] = team.ID
	if _, err := runStep(t, NewSetAdmin(), 1, st, 1, 1, Data{"id": player.ID}); err != nil {
		t.Fatal(err)
	}
	got, err = st.GetTeamByID(team.ID)
	if err != nil || len(got.Admins) != 2 {
		t.Fatalf("admin was not added: %+v %v", got, err)
	}
	other := domain.Player{ID: "other", TeamID: "another", TelegramID: 99}
	if err := st.CreatePlayer(&other); err != nil {
		t.Fatal(err)
	}
	if _, err := runStep(t, NewSetAdmin(), 1, st, 1, 1, Data{"id": other.ID}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("foreign player accepted: %v", err)
	}
}

type failingParticipation struct {
	storage.Storage
	failure error
}

func (s failingParticipation) AddTrainingPlayer(string, string) error    { return s.failure }
func (s failingParticipation) RemoveTrainingPlayer(string, string) error { return s.failure }

func TestParticipationFailureDoesNotProduceSuccessOrAskReason(t *testing.T) {
	st, _, player, training := scenarioFixture(t)
	failure := errors.New("database unavailable")
	failing := failingParticipation{st, failure}
	key := sessionKey{42, 42}
	userCancelFriendTrainingID[key] = training.ID
	userCancelContextMessage[key] = 10
	userFriendTrainingID[key] = training.ID
	userContextMessage[key] = 10
	for _, tc := range []struct {
		sc    Scenario
		index int
		data  Data
	}{
		{NewJoinTraining(), 6, Data{"training_id": training.ID}},
		{NewFriendJoinTraining(), 6, Data{"training_id": training.ID, "id": player.ID}},
		{NewCancelJoinTraining(), 0, Data{"id": training.ID}},
		{NewCancelFriendJoinTraining(), 1, Data{"id": player.ID}},
	} {
		user_state.ClearUserState(42, 42)
		action, err := runStep(t, tc.sc, tc.index, failing, 42, 42, tc.data)
		if !errors.Is(err, failure) || action != nil {
			t.Fatalf("failure hidden: action=%+v err=%v", action, err)
		}
		if strings.HasPrefix(user_state.GetUserState(42, 42), "cancel_reason:") {
			t.Fatal("asked for reason after failed cancellation")
		}
	}
}

func TestConcurrentScenarioDraftsAreIsolatedByUserAndChat(t *testing.T) {
	st, team, _, training := scenarioFixture(t)
	other := domain.Training{ID: "second", TeamID: team.ID, Team: team}
	if err := st.CreateTraining(&other); err != nil {
		t.Fatal(err)
	}
	first, _ := NewJoinTraining().GetStep(0)
	name, _ := NewJoinTraining().GetStep(1)
	var wg sync.WaitGroup
	for userID := int64(1000); userID < 1032; userID++ {
		for _, tr := range []domain.Training{training, other} {
			wg.Add(1)
			go func(userID int64, tr domain.Training) {
				defer wg.Done()
				chatID := int64(-1)
				if tr.ID == other.ID {
					chatID = -2
				}
				if _, err := first(st, chatID, userID, 10, Data{"id": tr.ID}); err != nil {
					t.Error(err)
					return
				}
				if _, err := name(st, chatID, userID, 11, Data{"message": tr.ID}); err != nil {
					t.Error(err)
				}
			}(userID, tr)
		}
	}
	wg.Wait()
	for userID := int64(1000); userID < 1032; userID++ {
		for chatID, id := range map[int64]string{-1: training.ID, -2: other.ID} {
			key := sessionKey{userID, chatID}
			if userTrainingID[key] != id || globalUserPlayer[key].FullName != id {
				t.Fatalf("drafts mixed for %+v", key)
			}
			delete(userTrainingID, key)
			delete(userTrainingMessageID, key)
			clearGlobalUserPlayer(userID, chatID)
			user_state.ClearUserState(userID, chatID)
		}
	}
}

func TestAdminCanCreateTrainingAndApproveLineup(t *testing.T) {
	st, team, _, _ := scenarioFixture(t)
	if _, err := runStep(t, NewScheduleTraining(), 0, st, 1, 1, Data{"team_id": team.ID}); err != nil {
		t.Fatal(err)
	}
	draft, err := getGlobalUserTraining(1, 1)
	if err != nil {
		t.Fatal(err)
	}
	draft.Date = time.Now().Add(time.Hour)
	draft.Type = domain.TrainingGaming
	draft.Capacity = 10
	saveGlobalUserTraining(1, 1, *draft)
	action, err := runStep(t, NewScheduleTraining(), 13, st, 1, 1, Data{"message": "До встречи"})
	if err != nil || len(action.Pin) != 1 {
		t.Fatalf("training not published: %+v %v", action, err)
	}
	created, err := st.GetTrainingByID(draft.ID)
	if err != nil || created == nil || created.Message != "До встречи" {
		t.Fatalf("training not saved: %+v %v", created, err)
	}
	game := domain.Training{ID: "game", TeamID: team.ID, Team: team, Date: time.Now().Add(time.Hour), Type: domain.TrainingMatch}
	for i := 0; i < 6; i++ {
		p := domain.Player{ID: fmt.Sprintf("lineup-%d", i), TeamID: team.ID, TelegramID: int64(100 + i), FullName: fmt.Sprint(i), Position: domain.PositionForward}
		if i == 0 {
			p.Position = domain.PositionGoalkeeper
		}
		if err := st.CreatePlayer(&p); err != nil {
			t.Fatal(err)
		}
		game.Players = append(game.Players, p)
	}
	if err := st.CreateTraining(&game); err != nil {
		t.Fatal(err)
	}
	if _, err := runStep(t, NewApproveLineup(), 0, st, 1, 1, Data{"ti": team.ID}); err != nil {
		t.Fatal(err)
	}
	action, err = runStep(t, NewApproveLineup(), 2, st, 1, 1, Data{"g": game.ID})
	if err != nil || action.CallbackAlert != "" || len(action.Pin) != 1 {
		t.Fatalf("approval failed: %+v %v", action, err)
	}
	approved, err := st.GetApprovedPlayerIDs(game.ID)
	if err != nil || len(approved) != 6 {
		t.Fatalf("lineup not saved: %v %v", approved, err)
	}
}
