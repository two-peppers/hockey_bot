package storage

import (
	"fmt"
	"hockey_bot/internal/domain"
	"time"

	"gorm.io/gorm"
)

func (s *gormStorage) CreateTraining(training *domain.Training) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Omit("Players").Create(training).Error; err != nil {
			return err
		}
		return saveTrainingPlayers(tx, training)
	})
}

// GetTrainingById возвращает команду по UUID
func (s *gormStorage) GetTrainingByID(id string) (*domain.Training, error) {
	var training domain.Training
	err := s.db.
		Preload("Players", func(db *gorm.DB) *gorm.DB {
			return db.Joins("JOIN training_players tp ON tp.player_id = players.id").
				Where("tp.training_id = ?", training.ID).
				Order("tp.signup_order ASC")
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

// SaveTraining сохраняет тренировку и список игроков (merge: добавляет новых, удаляет отсутствующих, не трогает approved у существующих).
func (s *gormStorage) SaveTraining(training *domain.Training) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		// Связи сохраняем явно: автоматическое сохранение many2many не
		// заполняет время и номер записи в нашей модели TrainingPlayer.
		if err := tx.Omit("Players").Save(training).Error; err != nil {
			return fmt.Errorf("failed to save training: %w", err)
		}
		return saveTrainingPlayers(tx, training)
	})
}

func saveTrainingPlayers(tx *gorm.DB, training *domain.Training) error {
	var existingIDs []string
	if err := tx.Model(&domain.TrainingPlayer{}).Where("training_id = ?", training.ID).Pluck("player_id", &existingIDs).Error; err != nil {
		return err
	}
	existingSet := make(map[string]bool)
	for _, id := range existingIDs {
		existingSet[id] = true
	}

	var lastOrder int64
	if err := tx.Model(&domain.TrainingPlayer{}).Where("training_id = ?", training.ID).
		Select("COALESCE(MAX(signup_order), 0)").Scan(&lastOrder).Error; err != nil {
		return err
	}
	for _, player := range training.Players {
		if !existingSet[player.ID] {
			lastOrder++
			tp := domain.TrainingPlayer{
				TrainingID:  training.ID,
				PlayerID:    player.ID,
				Approved:    false,
				SignupOrder: lastOrder,
			}
			if err := tx.Create(&tp).Error; err != nil {
				return fmt.Errorf("failed to add training_player: %w", err)
			}
			existingSet[player.ID] = true
		}
	}

	playerIDs := make([]string, 0, len(training.Players))
	for _, p := range training.Players {
		playerIDs = append(playerIDs, p.ID)
	}
	if len(playerIDs) > 0 {
		if err := tx.Where("training_id = ? AND player_id NOT IN ?", training.ID, playerIDs).Delete(&domain.TrainingPlayer{}).Error; err != nil {
			return fmt.Errorf("failed to remove old training_players: %w", err)
		}
	} else {
		if err := tx.Where("training_id = ?", training.ID).Delete(&domain.TrainingPlayer{}).Error; err != nil {
			return fmt.Errorf("failed to clear training_players: %w", err)
		}
	}

	return nil
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
	return list, sortTrainingPlayers(s.db, list)
}

// GetUpcomingTrainingsByTelegramID возвращает предстоящие тренировки пользователя (кроме матчей) во всех его командах.
func (s *gormStorage) GetUpcomingTrainingsByTelegramID(telegramID int64, limit int) ([]domain.Training, error) {
	if limit <= 0 {
		limit = 10
	}
	var list []domain.Training
	now := time.Now()

	// Выбираем все тренировки команд, в которых у пользователя есть игрок (по telegram_id),
	// независимо от того, записан ли он на конкретную тренировку.
	subQuery := s.db.Model(&domain.Player{}).
		Select("team_id").
		Where("telegram_id = ?", telegramID)

	err := s.db.
		Model(&domain.Training{}).
		Preload("Players").
		Preload("Team").
		Where("team_id IN (?) AND type <> ? AND date >= ?", subQuery, domain.TrainingMatch, now).
		Order("date ASC").
		Limit(limit).
		Find(&list).Error
	if err != nil {
		return nil, err
	}
	return list, sortTrainingPlayers(s.db, list)
}

// GetApprovedPlayerIDs возвращает утверждённых участников в порядке записи.
func (s *gormStorage) GetApprovedPlayerIDs(trainingID string) ([]string, error) {
	var ids []string
	err := s.db.Model(&domain.TrainingPlayer{}).
		Where("training_id = ? AND approved = ?", trainingID, true).
		Order("signup_order ASC").
		Pluck("player_id", &ids).Error
	return ids, err
}

// SetApprovedPlayers выставляет approved только для переданных player_id и обновляет capacity тренировки.
func (s *gormStorage) SetApprovedPlayers(trainingID string, approvedPlayerIDs []string) error {
	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	if err := tx.Model(&domain.TrainingPlayer{}).
		Where("training_id = ?", trainingID).
		Update("approved", false).Error; err != nil {
		tx.Rollback()
		return err
	}
	if len(approvedPlayerIDs) > 0 {
		if err := tx.Model(&domain.TrainingPlayer{}).
			Where("training_id = ? AND player_id IN ?", trainingID, approvedPlayerIDs).
			Update("approved", true).Error; err != nil {
			tx.Rollback()
			return err
		}
	}
	capacity := len(approvedPlayerIDs)
	if capacity < 1 {
		capacity = 1
	}
	if err := tx.Model(&domain.Training{}).Where("id = ?", trainingID).Update("capacity", capacity).Error; err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit().Error
}

// UpdateTeamMessageID сохраняет message_id исходного сообщения тренировки/игры в командном чате.
func (s *gormStorage) UpdateTeamMessageID(trainingID string, messageID int) error {
	return s.db.Model(&domain.Training{}).
		Where("id = ?", trainingID).
		Update("team_message_id", messageID).Error
}

// UpdateApprovedMessageID сохраняет message_id сообщения с утверждённым составом для тренировки/игры.
func (s *gormStorage) UpdateApprovedMessageID(trainingID string, messageID int) error {
	return s.db.Model(&domain.Training{}).
		Where("id = ?", trainingID).
		Update("approved_message_id", messageID).Error
}
