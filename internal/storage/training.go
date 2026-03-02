package storage

import (
	"fmt"
	"hockey_bot/internal/domain"
	"time"

	"gorm.io/gorm"
)

func (s *gormStorage) CreateTraining(training *domain.Training) error {
	return s.db.Create(&training).Error
}

// GetTrainingById возвращает команду по UUID
func (s *gormStorage) GetTrainingByID(id string) (*domain.Training, error) {
	var training domain.Training
	err := s.db.
		Preload("Players", func(db *gorm.DB) *gorm.DB {
			return db.Joins("JOIN training_players tp ON tp.player_id = players.id").
				Where("tp.training_id = ?", training.ID).
				Order("tp.created_at ASC")
		}).
		Preload("Team").
		Where("id = ?", id).
		First(&training).Error
	if err != nil {
		if gorm.ErrRecordNotFound == err {
			return nil, nil
		}
		return nil, err
	}
	return &training, nil
}

// SaveTraining сохраняет тренировку с текущим списком игроков
func (s *gormStorage) SaveTraining(training *domain.Training) error {
	// 1. Начинаем транзакцию для атомарности
	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	// 2. Сохраняем саму тренировку
	if err := tx.Save(training).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to save training: %w", err)
	}

	// 3. Удаляем старые связи TrainingPlayer
	if err := tx.Exec("DELETE FROM training_players WHERE training_id = ?", training.ID).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to clear old associations: %w", err)
	}

	// 4. Создаем новые связи для текущих игроков
	for _, player := range training.Players {
		trainingPlayer := domain.TrainingPlayer{
			TrainingID: training.ID,
			PlayerID:   player.ID,
		}
		if err := tx.Create(&trainingPlayer).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to save training_player: %w", err)
		}
	}

	// 5. Коммитим транзакцию
	return tx.Commit().Error
}

func (s *gormStorage) GetRecentVenues(teamID string) []string {
	var venues []string

	err := s.db.
		Model(&domain.Training{}).
		Select("DISTINCT venue").
		Where("venue IS NOT NULL AND venue != '' AND team_id = ?", teamID).
		Order("created_at DESC").
		Limit(10).
		Pluck("venue", &venues).Error

	if err != nil {
		fmt.Printf("failed to get recent venues: %v", err)
	}

	return venues
}

// GetUpcomingGames возвращает ближайшие предстоящие игры команды (type=match, date >= now).
func (s *gormStorage) GetUpcomingGames(teamID string, limit int) ([]domain.Training, error) {
	if limit <= 0 {
		limit = 10
	}
	var list []domain.Training
	now := time.Now()
	err := s.db.
		Preload("Players").
		Preload("Team").
		Where("team_id = ? AND type = ? AND date >= ?", teamID, domain.TrainingMatch, now).
		Order("date ASC").
		Limit(limit).
		Find(&list).Error
	if err != nil {
		return nil, err
	}
	return list, nil
}
