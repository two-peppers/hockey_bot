package storage

import (
	"hockey_bot/internal/domain"

	"gorm.io/gorm"
)

func (s *gormStorage) GetTeamsWithChat() ([]domain.Team, error) {
	var teams []domain.Team
	err := s.db.Where("chat_id IS NOT NULL").Find(&teams).Error
	return teams, err
}

func (s *gormStorage) CreatePlayer(player *domain.Player) error {
	return s.db.Create(player).Error
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
