package domain

import (
	"fmt"
	"strings"
	"time"
)

type TrainingType string

const (
	TrainingGaming    TrainingType = "gaming"    // Игровая
	TrainingTechnical TrainingType = "technical" // Техническая
	TrainingShooting  TrainingType = "shooting"  // Бросковая
	TrainingChecking  TrainingType = "checking"  // Подкатка
	TrainingMatch     TrainingType = "match"     // Игра в федерации
)

func (t TrainingType) ToString() string {
	switch t {
	case TrainingGaming:
		return "Тренировка игровая"
	case TrainingTechnical:
		return "Тренировка"
	case TrainingShooting:
		return "Броски на сухом льду"
	case TrainingChecking:
		return "Подкатка"
	case TrainingMatch:
		return "Игра на федерацию в рамках регулярного первенства"
	default:
		return "Неизвестно"
	}
}

type Training struct {
	ID             string       `gorm:"primaryKey;type:text;size:36" json:"id"`
	TeamID         string       `gorm:"index" json:"team_id"`
	Team           Team         `gorm:"foreignKey:TeamID;references:ID"`
	Type           TrainingType `gorm:"type:varchar(20);index" json:"type"`
	Date           time.Time    `gorm:"index" json:"date"`
	Venue          string       `json:"venue"`
	Opponent       string       `json:"opponent"`
	Division       string       `json:"division"`
	Capacity       int          `gorm:"index" json:"capacity"`
	StrictCapacity bool         `json:"strict_capacity"`
	Players        []Player     `gorm:"many2many:training_players;foreignKey:ID;joinForeignKey:TrainingID;References:ID;joinReferences:PlayerID"` // ✅ Many-to-Many
	Message        string       `gorm:"size:255" json:"message"`
	CreatedAt      time.Time    `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time    `gorm:"autoUpdateTime" json:"updated_at"`
}

// TrainingPlayer Связующая таблица (GORM создаст автоматически)
type TrainingPlayer struct {
	TrainingID string    `gorm:"primaryKey"`
	PlayerID   string    `gorm:"primaryKey"`
	CreatedAt  time.Time `gorm:"autoCreateTime"`
}

func (t *Training) String() string {
	var goalkeepers []string
	var players []string
	var message string

	weekdays := []string{
		"Воскресенье",
		"Понедельник",
		"Вторник",
		"Среда",
		"Четверг",
		"Пятница",
		"Суббота",
	}

	fpc := 0
	for _, player := range t.Players {
		if player.Position == PositionGoalkeeper {
			goalkeepers = append(goalkeepers, fmt.Sprintf("#вратарь %s", player.FullName))
		} else {
			fpc++
			players = append(players, fmt.Sprintf("%d. %s", fpc, player.FullName))
		}
	}

	if len(players) < t.Capacity {
		for i := len(players); i < t.Capacity; i++ {
			players = append(players, fmt.Sprintf("%d.", i+1))
		}
	}

	if len(goalkeepers) < 2 {
		for i := len(goalkeepers); i < 2; i++ {
			goalkeepers = append(goalkeepers, "#вратарь")
		}
	}

	if t.Type == TrainingMatch {
		message = "❗ Внимание ❗\n" +
			"Игра по Дивизиону " + t.Division + "\n\n" +
			"📅 " + weekdays[t.Date.Weekday()] + ", " + t.Date.Format("02.01") + "\n\n" +
			t.Type.ToString() + " против «" + t.Opponent + "»\n\n" +
			"⏰ " + t.Date.Format("15:04") + "\n" +
			"📍 " + t.Venue + "\n\n" +
			"Сейчас формируем список желающих!\n\nОтталкиваясь от этого списка, формируется состав на игру!\n\n"
		message += strings.Join(goalkeepers, "\n") + "\n\n"
		message += strings.Join(players, "\n") + "\n\n"
		message += t.Message
	} else {
		message = t.Date.Format("02.01") + "\n" +
			weekdays[t.Date.Weekday()] + "\n" +
			t.Type.ToString() + "\n\n" +
			"⏰ " + t.Date.Format("15:04") + "\n" +
			"📍 " + t.Venue + "\n\n"

		message += strings.Join(goalkeepers, "\n") + "\n\n"
		message += strings.Join(players, "\n") + "\n\n"
		message += t.Message
	}

	return message
}

func (t *Training) AddPlayer(player *Player) {
	for _, teamPlayer := range t.Players {
		if teamPlayer.ID == player.ID {
			return
		}
	}

	t.Players = append(t.Players, *player)
}

func (t *Training) RemovePlayer(player *Player) {
	newPlayers := make([]Player, 0)
	for _, p := range t.Players {
		if p.ID != player.ID {
			newPlayers = append(newPlayers, p)
		}
	}

	t.Players = newPlayers
}
