package domain

import (
	"time"
)

// PlayerPosition - енум амплуа
type PlayerPosition string

const (
	PositionGoalkeeper PlayerPosition = "goalkeeper" // Вр
	PositionForward    PlayerPosition = "forward"    // Нп
	PositionDefender   PlayerPosition = "defender"   // Зщ
	PositionTrainer    PlayerPosition = "trainer"    // Тр
)

func (p PlayerPosition) ToString() string {
	switch p {
	case PositionGoalkeeper:
		return "Вр"
	case PositionForward:
		return "Нп"
	case PositionDefender:
		return "Зщ"
	case PositionTrainer:
		return "Тр"
	default:
		return "?"
	}
}

// Player - модель игрока команды
type Player struct {
	ID         string         `gorm:"primaryKey;size:36" json:"id"`
	TeamID     string         `gorm:"index" json:"team_id"`      // ссылка на команду
	TelegramID int64          `gorm:"index" json:"telegram_id"`  // уникальный ID игрока
	FullName   string         `gorm:"size:100" json:"full_name"` // "Иванов И.И."
	Position   PlayerPosition `gorm:"type:varchar(20);index" json:"position"`
	Commercial bool           `gorm:"type:bool" json:"commercial"` // Коммерческий игрок (только для вратарей)
	CreatedAt  time.Time      `gorm:"autoCreateTime" json:"created_at"`
}
