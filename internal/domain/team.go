// Пакет с моделями домена (БД)
// Пока пустой - добавим модели команд позже
package domain

import (
	"time"
)

type Team struct {
	ID        string    `gorm:"primaryKey;size:36" json:"id"` // UUID как строка
	Name      string    `gorm:"uniqueIndex;not null;size:255" json:"name"`
	ChatID    *int64    `gorm:"index" json:"chat_id,omitempty"`
	Admins    []int64   `gorm:"type:json;serializer:json" json:"admins"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}
