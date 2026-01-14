package storage

import (
	"hockey_bot/internal/domain"

	"gorm.io/gorm"
)

// CreateTeam создает команду с заданным именем
func (s *gormStorage) CreateTeam(team *domain.Team) error {
	return s.db.Create(team).Error
}

func (s *gormStorage) SaveTeam(team *domain.Team) error {
	seen := make(map[int64]struct{})
	filteredAdmins := make([]int64, 0)

	for _, admin := range team.Admins {
		if _, ok := seen[admin]; ok {
			continue
		}
		filteredAdmins = append(filteredAdmins, admin)
		seen[admin] = struct{}{}
	}

	team.Admins = filteredAdmins

	return s.db.Save(team).Error
}

// GetTeamByName возвращает команду по названию
func (s *gormStorage) GetTeamByName(name string) (*domain.Team, error) {
	var team domain.Team
	err := s.db.Where("name = ?", name).First(&team).Error
	if err != nil {
		if gorm.ErrRecordNotFound == err {
			return nil, nil // команда не найдена - нормально
		}
		return nil, err
	}
	return &team, nil
}

// GetTeamByID возвращает команду по UUID
func (s *gormStorage) GetTeamByID(id string) (*domain.Team, error) {
	var team domain.Team
	err := s.db.Where("id = ?", id).First(&team).Error
	if err != nil {
		if gorm.ErrRecordNotFound == err {
			return nil, nil
		}
		return nil, err
	}
	return &team, nil
}

// UpdateTeamChatID обновляет chat_id команды
func (s *gormStorage) UpdateTeamChatID(id string, chatID int64) error {
	return s.db.Model(&domain.Team{}).Where("id = ?", id).Update("chat_id", chatID).Error
}

// IsAdmin проверяет, является ли user_id админом команды
func (s *gormStorage) IsAdmin(teamID string, userID int64) (bool, error) {
	team, err := s.GetTeamByID(teamID)
	if err != nil {
		return false, err
	}
	if team == nil {
		return false, nil
	}

	for _, adminID := range team.Admins {
		if adminID == userID {
			return true, nil
		}
	}
	return false, nil
}

// GetTeamsWithoutChatByAdmin Находит команды пользователя (админа) без chat_id
func (s *gormStorage) GetTeamsWithoutChatByAdmin(userID int64) ([]domain.Team, error) {
	var teams []domain.Team

	err := s.db.Where("chat_id IS NULL").Find(&teams).Error
	if err != nil {
		return nil, err
	}

	var userTeams []domain.Team
	for _, team := range teams {
		for _, adminID := range team.Admins {
			if adminID == userID {
				userTeams = append(userTeams, team)
				break // нашли - добавляем команду
			}
		}
	}

	return userTeams, nil
}

// GetTeamsByAdmin возвращает команды, где userID является админом
func (s *gormStorage) GetTeamsByAdmin(userID int64) ([]domain.Team, error) {
	var teams []domain.Team

	err := s.db.Find(&teams).Error
	if err != nil {
		return nil, err
	}

	var adminTeams []domain.Team
	for _, team := range teams {
		for _, adminID := range team.Admins {
			if adminID == userID {
				adminTeams = append(adminTeams, team)
				break
			}
		}
	}
	return adminTeams, nil
}

func (s *gormStorage) GetTeamByChatID(chatID int64) (*domain.Team, error) {
	var team domain.Team

	err := s.db.Where("chat_id = ?", chatID).
		First(&team).Error

	if err != nil {
		return nil, err
	}

	return &team, nil
}
