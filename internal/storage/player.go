package storage

import (
	"fmt"
	"hockey_bot/internal/domain"
	"strings"

	"gorm.io/gorm"
)

func (s *gormStorage) GetTeamsWithChat() ([]domain.Team, error) {
	var teams []domain.Team
	err := s.db.Where("chat_id IS NOT NULL").Find(&teams).Error
	return teams, err
}

func (s *gormStorage) CreatePlayer(player *domain.Player) error {
	// Повторный ручной ввод уже известного друга не должен создавать ещё одну
	// строку. Возвращаем ID существующей записи, чтобы вызывающий код мог сразу
	// добавить её в тренировку.
	var existing []domain.Player
	err := s.db.Where("team_id = ? AND telegram_id = ?", player.TeamID, player.TelegramID).
		Order("created_at ASC").Find(&existing).Error
	if err != nil {
		return err
	}
	for _, candidate := range existing {
		if strings.EqualFold(strings.TrimSpace(candidate.FullName), strings.TrimSpace(player.FullName)) &&
			candidate.Position == player.Position && candidate.Commercial == player.Commercial {
			player.ID = candidate.ID
			player.CreatedAt = candidate.CreatedAt
			return nil
		}
	}
	return s.db.Create(player).Error
}

// migrateDuplicateFriends удаляет повторные записи друзей и переносит их
// связи с тренировками на самую старую запись. Первая запись пользователя в
// каждой команде считается самим пользователем и никогда не удаляется.
func migrateDuplicateFriends(db *gorm.DB) (int, error) {
	var players []domain.Player
	if err := db.Order("team_id, telegram_id, created_at, id").Find(&players).Error; err != nil {
		return 0, err
	}

	tx := db.Begin()
	if tx.Error != nil {
		return 0, tx.Error
	}
	removed := 0
	group := ""
	seen := make(map[string]string)
	for _, player := range players {
		playerGroup := player.TeamID + ":" + fmt.Sprint(player.TelegramID)
		if playerGroup != group {
			group = playerGroup
			seen = make(map[string]string)
			// Сам игрок — первая и самая старая запись владельца в команде.
			continue
		}
		key := strings.ToLower(strings.TrimSpace(player.FullName)) + ":" + string(player.Position) + ":" + fmt.Sprint(player.Commercial)
		canonicalID, duplicate := seen[key]
		if !duplicate {
			seen[key] = player.ID
			continue
		}

		var links []domain.TrainingPlayer
		if err := tx.Where("player_id = ?", player.ID).Find(&links).Error; err != nil {
			tx.Rollback()
			return removed, err
		}
		for _, link := range links {
			var canonical domain.TrainingPlayer
			err := tx.Where("training_id = ? AND player_id = ?", link.TrainingID, canonicalID).First(&canonical).Error
			if err == gorm.ErrRecordNotFound {
				link.PlayerID = canonicalID
				if err := tx.Create(&link).Error; err != nil {
					tx.Rollback()
					return removed, err
				}
			} else if err != nil {
				tx.Rollback()
				return removed, err
			} else if link.Approved && !canonical.Approved {
				if err := tx.Model(&canonical).Update("approved", true).Error; err != nil {
					tx.Rollback()
					return removed, err
				}
			}
		}
		if err := tx.Where("player_id = ?", player.ID).Delete(&domain.TrainingPlayer{}).Error; err != nil {
			tx.Rollback()
			return removed, err
		}
		if err := tx.Delete(&player).Error; err != nil {
			tx.Rollback()
			return removed, err
		}
		removed++
	}
	return removed, tx.Commit().Error
}

func (s *gormStorage) PlayerExists(teamID string, telegramID int64) (bool, error) {
	var count int64
	err := s.db.Model(&domain.Player{}).
		Where("team_id = ? AND telegram_id = ?", teamID, telegramID).
		Count(&count).Error
	return count > 0, err
}

func (s *gormStorage) GetPlayer(id int64, teamID string) (*domain.Player, error) {
	var player domain.Player
	err := s.db.Where("telegram_id = ? AND team_id = ?", id, teamID).
		Order("created_at ASC").
		First(&player).Error
	if err != nil {
		if gorm.ErrRecordNotFound == err {
			return nil, nil
		}
		return nil, err
	}
	return &player, nil
}

func (s *gormStorage) GetPlayerByID(id string) (*domain.Player, error) {
	var player domain.Player

	err := s.db.
		Where("id = ?", id).
		First(&player).Error

	if err != nil {
		if gorm.ErrRecordNotFound == err {
			return nil, nil
		}
		return nil, err
	}

	return &player, nil
}

func (s *gormStorage) GetPlayersByTelegramID(teamID string, telegramID int64) ([]domain.Player, error) {
	var players []domain.Player

	err := s.db.Where("telegram_id = ? AND team_id = ?", telegramID, teamID).
		Order("created_at ASC").
		Find(&players).
		Error

	if err != nil {
		return nil, err
	}

	return players, nil
}

func (s *gormStorage) GetPlayersByTeamID(teamID string) ([]domain.Player, error) {
	var players []domain.Player

	err := s.db.Where("team_id = ?", teamID).
		Order("created_at ASC").
		Find(&players).Error

	if err != nil {
		return nil, err
	}

	return players, nil
}
