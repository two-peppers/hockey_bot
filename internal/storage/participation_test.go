package storage

import (
	"fmt"
	"hockey_bot/internal/domain"
	"sync"
	"testing"
)

func TestConcurrentParticipationPreservesOtherPlayersAndApproval(t *testing.T) {
	store := newTrainingTestStorage(t)
	team := domain.Team{ID: "team", Name: "Команда"}
	if err := store.CreateTeam(&team); err != nil {
		t.Fatal(err)
	}
	players := make([]domain.Player, 32)
	for i := range players {
		players[i] = domain.Player{ID: fmt.Sprint(i), TeamID: team.ID}
	}
	if err := store.db.Create(&players).Error; err != nil {
		t.Fatal(err)
	}
	training := domain.Training{ID: "training", TeamID: team.ID, Players: players[:1]}
	if err := store.CreateTraining(&training); err != nil {
		t.Fatal(err)
	}
	if err := store.SetApprovedPlayers(training.ID, []string{players[0].ID}); err != nil {
		t.Fatal(err)
	}
	// Все пользователи могли прочитать исходный состав до записи любого из них.
	var wg sync.WaitGroup
	for _, p := range players {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			if err := store.AddTrainingPlayer(training.ID, id); err != nil {
				t.Error(err)
			}
		}(p.ID)
	}
	wg.Wait()
	got, err := store.GetTrainingByID(training.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Players) != len(players) {
		t.Fatalf("got %d players, want %d", len(got.Players), len(players))
	}
	approved, err := store.GetApprovedPlayerIDs(training.ID)
	if err != nil || len(approved) != 1 || approved[0] != players[0].ID {
		t.Fatalf("approval lost: %v, %v", approved, err)
	}
	var links []domain.TrainingPlayer
	if err := store.db.Where("training_id = ?", training.ID).Find(&links).Error; err != nil {
		t.Fatal(err)
	}
	orders := map[int64]bool{}
	for _, link := range links {
		if orders[link.SignupOrder] {
			t.Fatal("duplicate signup order")
		}
		orders[link.SignupOrder] = true
	}
	for _, p := range players[1:16] {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			if err := store.RemoveTrainingPlayer(training.ID, id); err != nil {
				t.Error(err)
			}
		}(p.ID)
	}
	// Повторная запись оставшихся игроков не должна восстанавливать удалённых.
	for _, p := range players[16:] {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			if err := store.AddTrainingPlayer(training.ID, id); err != nil {
				t.Error(err)
			}
		}(p.ID)
	}
	wg.Wait()
	got, err = store.GetTrainingByID(training.ID)
	if err != nil || len(got.Players) != 17 {
		t.Fatalf("unexpected final roster: %+v, %v", got, err)
	}
	if err := store.AddTrainingPlayer(training.ID, players[1].ID); err != nil {
		t.Fatal(err)
	}
	got, err = store.GetTrainingByID(training.ID)
	if err != nil || got.Players[len(got.Players)-1].ID != players[1].ID {
		t.Fatal("rejoining player did not go to the end")
	}
}

func TestParticipationRejectsMissingAndForeignPlayers(t *testing.T) {
	store := newTrainingTestStorage(t)
	training := domain.Training{ID: "training", TeamID: "team"}
	if err := store.CreateTraining(&training); err != nil {
		t.Fatal(err)
	}
	if err := store.db.Create(&domain.Player{ID: "foreign", TeamID: "another"}).Error; err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"foreign", "missing"} {
		if err := store.AddTrainingPlayer(training.ID, id); err == nil {
			t.Fatalf("accepted player %q", id)
		}
	}
	if err := store.AddTrainingPlayer("missing", "foreign"); err == nil {
		t.Fatal("accepted missing training")
	}
}
