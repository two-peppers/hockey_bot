package storage

import (
	"hockey_bot/internal/domain"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestMigrateDuplicateFriends(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&domain.Team{}, &domain.Player{}, &domain.Training{}, &domain.TrainingPlayer{}); err != nil {
		t.Fatal(err)
	}

	createdAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	players := []domain.Player{
		{ID: "owner", TeamID: "team", TelegramID: 42, FullName: "Тренер", Position: domain.PositionTrainer, CreatedAt: createdAt},
		{ID: "friend", TeamID: "team", TelegramID: 42, FullName: "Иванов", Position: domain.PositionForward, CreatedAt: createdAt.Add(time.Second)},
		{ID: "duplicate", TeamID: "team", TelegramID: 42, FullName: " иВаНоВ ", Position: domain.PositionForward, CreatedAt: createdAt.Add(2 * time.Second)},
	}
	for i := range players {
		if err := db.Create(&players[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&domain.TrainingPlayer{TrainingID: "training", PlayerID: "duplicate", Approved: true}).Error; err != nil {
		t.Fatal(err)
	}

	removed, err := migrateDuplicateFriends(db)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}

	var remaining []domain.Player
	if err := db.Order("created_at").Find(&remaining).Error; err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 2 || remaining[0].ID != "owner" || remaining[1].ID != "friend" {
		t.Fatalf("remaining players = %#v", remaining)
	}
	var link domain.TrainingPlayer
	if err := db.Where("training_id = ? AND player_id = ?", "training", "friend").First(&link).Error; err != nil {
		t.Fatalf("canonical training link not found: %v", err)
	}
	if !link.Approved {
		t.Fatal("approved flag was not preserved")
	}

	removed, err = migrateDuplicateFriends(db)
	if err != nil || removed != 0 {
		t.Fatalf("second migration: removed=%d err=%v", removed, err)
	}
}

func TestCreatePlayerReusesExistingFriend(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&domain.Player{}); err != nil {
		t.Fatal(err)
	}
	store := &gormStorage{db: db}
	existing := domain.Player{ID: "friend", TeamID: "team", TelegramID: 42, FullName: "Иванов", Position: domain.PositionForward}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatal(err)
	}

	duplicate := domain.Player{ID: "new-id", TeamID: "team", TelegramID: 42, FullName: "  иванов ", Position: domain.PositionForward}
	if err := store.CreatePlayer(&duplicate); err != nil {
		t.Fatal(err)
	}
	if duplicate.ID != existing.ID {
		t.Fatalf("player ID = %q, want existing ID %q", duplicate.ID, existing.ID)
	}
	var count int64
	if err := db.Model(&domain.Player{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("players count = %d, want 1", count)
	}
}
