package storage

import (
	"hockey_bot/internal/domain"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newTrainingTestStorage(t *testing.T) *gormStorage {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		NowFunc: func() time.Time { return time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&domain.Team{}, &domain.Player{}, &domain.Training{}, &domain.TrainingPlayer{}); err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.Close() })
	return &gormStorage{db: db}
}

func TestGetTrainingByIDPreservesSignupOrder(t *testing.T) {
	store := newTrainingTestStorage(t)
	db := store.db
	team := domain.Team{ID: "team", Name: "Команда"}
	training := domain.Training{ID: "training", TeamID: team.ID}
	players := []domain.Player{
		{ID: "z-first", TeamID: team.ID, FullName: "Первый"},
		{ID: "a-second", TeamID: team.ID, FullName: "Второй"},
		{ID: "m-third", TeamID: team.ID, FullName: "Третий"},
	}
	if err := db.Create(&team).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&training).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&players).Error; err != nil {
		t.Fatal(err)
	}

	// Одинаковое время воспроизводит случай, когда сортировка только по
	// created_at не определяет порядок участников однозначно.
	for _, player := range players {
		training.AddPlayer(&player)
		if err := store.SaveTraining(&training); err != nil {
			t.Fatal(err)
		}
	}

	got, err := store.GetTrainingByID(training.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Players) != len(players) {
		t.Fatalf("players count = %d, want %d", len(got.Players), len(players))
	}
	for i := range players {
		if got.Players[i].ID != players[i].ID {
			t.Fatalf("player %d = %q, want %q", i, got.Players[i].ID, players[i].ID)
		}
	}
}

func assertTrainingOrder(t *testing.T, store *gormStorage, id string, want []string) {
	t.Helper()
	training, err := store.GetTrainingByID(id)
	if err != nil || training == nil {
		t.Fatalf("GetTrainingByID(%q): training=%v, err=%v", id, training, err)
	}
	got := make([]string, 0, len(training.Players))
	for _, player := range training.Players {
		got = append(got, player.ID)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("player order = %v, want %v", got, want)
	}
}

func TestTrainingSignupOrderSurvivesSaveAndRejoin(t *testing.T) {
	store := newTrainingTestStorage(t)
	players := []domain.Player{{ID: "z"}, {ID: "a"}, {ID: "m"}}
	if err := store.db.Create(&players).Error; err != nil {
		t.Fatal(err)
	}
	training := domain.Training{ID: "training", Players: players}
	if err := store.CreateTraining(&training); err != nil {
		t.Fatal(err)
	}
	assertTrainingOrder(t, store, training.ID, []string{"z", "a", "m"})
	if err := store.SetApprovedPlayers(training.ID, []string{"m", "z", "a"}); err != nil {
		t.Fatal(err)
	}
	// Повторное сохранение и перестановка входного списка не меняют очередь.
	training.Players = []domain.Player{players[2], players[1], players[0]}
	if err := store.SaveTraining(&training); err != nil {
		t.Fatal(err)
	}
	assertTrainingOrder(t, store, training.ID, []string{"z", "a", "m"})
	approved, err := store.GetApprovedPlayerIDs(training.ID)
	if err != nil || !reflect.DeepEqual(approved, []string{"z", "a", "m"}) {
		t.Fatalf("approved = %v, err = %v", approved, err)
	}
	training.RemovePlayer(&players[0])
	if err := store.SaveTraining(&training); err != nil {
		t.Fatal(err)
	}
	assertTrainingOrder(t, store, training.ID, []string{"a", "m"})
	// Даже перевод часов назад не должен возвращать игрока в начало.
	store.db.Config.NowFunc = func() time.Time { return time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC) }
	training.AddPlayer(&players[0])
	if err := store.SaveTraining(&training); err != nil {
		t.Fatal(err)
	}
	assertTrainingOrder(t, store, training.ID, []string{"a", "m", "z"})
	approved, err = store.GetApprovedPlayerIDs(training.ID)
	if err != nil || !reflect.DeepEqual(approved, []string{"a", "m"}) {
		t.Fatalf("approved after rejoin = %v, err = %v", approved, err)
	}
	training.Players = nil
	if err := store.SaveTraining(&training); err != nil {
		t.Fatal(err)
	}
	assertTrainingOrder(t, store, training.ID, []string{})
	training.AddPlayer(&players[1])
	if err := store.SaveTraining(&training); err != nil {
		t.Fatal(err)
	}
	assertTrainingOrder(t, store, training.ID, []string{"a"})
}

func TestUpcomingTrainingPlayersFollowEachSignupOrder(t *testing.T) {
	store := newTrainingTestStorage(t)
	players := []domain.Player{{ID: "z", TeamID: "team", TelegramID: 42}, {ID: "a", TeamID: "team"}}
	if err := store.db.Create(&players).Error; err != nil {
		t.Fatal(err)
	}
	for _, typ := range []domain.TrainingType{domain.TrainingMatch, domain.TrainingGaming} {
		for i, id := range []string{"first", "second"} {
			ordered := []domain.Player{players[i], players[1-i]}
			training := domain.Training{ID: string(typ) + id, TeamID: "team", Type: typ,
				Date: time.Now().Add(time.Duration(i+1) * time.Hour), Players: ordered}
			if err := store.CreateTraining(&training); err != nil {
				t.Fatal(err)
			}
		}
	}
	check := func(trainings []domain.Training, err error) {
		t.Helper()
		if err != nil || len(trainings) != 2 {
			t.Fatalf("trainings count = %d, err = %v", len(trainings), err)
		}
		for i, training := range trainings {
			if len(training.Players) != 2 || training.Players[0].ID != players[i].ID || training.Players[1].ID != players[1-i].ID {
				t.Fatalf("unexpected player order for %s: %+v", training.ID, training.Players)
			}
		}
	}
	check(store.GetUpcomingGames("team", 10))
	check(store.GetUpcomingTrainingsByTelegramID(42, 10))
}

func TestMigrateLegacyTrainingSignupOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	// Схема до добавления signup_order, включая незаполненное created_at.
	if err := db.Exec(`CREATE TABLE training_players (
		training_id TEXT, player_id TEXT, approved NUMERIC DEFAULT false,
		created_at DATETIME, PRIMARY KEY (training_id, player_id))`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO training_players (training_id, player_id, approved)
		VALUES ('training', 'z', true), ('training', 'a', true), ('training', 'm', true)`).Error; err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	store := NewGORM(path).(*gormStorage)
	t.Cleanup(store.Close)
	want := []string{"z", "a", "m"}
	for i := 0; i < 2; i++ {
		got, err := store.GetApprovedPlayerIDs("training")
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("migrated order = %v, err = %v", got, err)
		}
		if err := migrateTrainingSignupOrder(store.db); err != nil {
			t.Fatal(err)
		}
	}
}
