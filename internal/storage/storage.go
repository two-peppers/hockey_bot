package storage

import (
	"hockey_bot/internal/domain"
	"log"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// Storage - интерфейс для работы с данными
// Позволит легко поменять SQLite на PostgreSQL
type Storage interface {
	// Close закрывает соединение с БД
	Close()
	GetTeamByName(name string) (*domain.Team, error)
	GetTeamByID(id string) (*domain.Team, error)
	CreateTeam(team *domain.Team) error
	SaveTeam(team *domain.Team) error
	UpdateTeamChatID(id string, chatID int64) error
	GetTeamsWithoutChatByAdmin(userID int64) ([]domain.Team, error)
	GetTeamsByAdmin(userID int64) ([]domain.Team, error)
	GetTeamByChatID(chatID int64) (*domain.Team, error)
	GetTeamsWithChat() ([]domain.Team, error)
	CreatePlayer(player *domain.Player) error
	PlayerExists(teamID string, telegramID int64) (bool, error)
	CreateTraining(training *domain.Training) error
	GetTrainingByID(id string) (*domain.Training, error)
	GetRecentVenues(teamID string) []string
	GetPlayer(id int64, teamId string) (*domain.Player, error)
	GetPlayerByID(id string) (*domain.Player, error)
	GetPlayersByTelegramID(teamId string, telegramID int64) ([]domain.Player, error)
	GetPlayersByTeamID(teamId string) ([]domain.Player, error)
	SaveTraining(training *domain.Training) error
}

// gormStorage - конкретная реализация на GORM+SQLite
type gormStorage struct {
	db *gorm.DB
}

// NewGORM создает хранилище на базе GORM+SQLite
func NewGORM(dbPath string) Storage {
	// Подключаемся к SQLite
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}

	// Включаем логи SQL запросов (удобно для дебага)
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("failed to get sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1) // SQLite не любит много соединений

	// Автомиграция - создает/обновляет таблицы
	// Пока пусто - добавим модели в domain и вызовем здесь
	err = db.AutoMigrate(&domain.Team{}, &domain.Player{}, &domain.Training{})
	if err != nil {
		log.Fatalf("failed to auto-migrate: %v", err)
	}

	log.Printf("SQLite connected: %s", dbPath)
	return &gormStorage{db: db}
}

// Close реализует интерфейс Storage
func (s *gormStorage) Close() {
	sqlDB, err := s.db.DB()
	if err != nil {
		log.Printf("warning: failed to get sql.DB: %v", err)
		return
	}
	sqlDB.Close()
}
