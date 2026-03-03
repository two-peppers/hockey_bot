package scenario

import (
	"errors"
	"hockey_bot/internal/domain"
	"hockey_bot/internal/pendingreply"
	"hockey_bot/internal/storage"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Data map[string]string

func (d *Data) Get(key string) string {
	if val, ok := (*d)[key]; ok {
		return val
	}

	return ""
}

func (d *Data) Set(key, value string) {
	(*d)[key] = value
}

type Action struct {
	Reply                        []*tgbotapi.MessageConfig
	Edit                         []*tgbotapi.EditMessageTextConfig
	Delete                       []*tgbotapi.DeleteMessageConfig
	Pin                          []*tgbotapi.MessageConfig
	TemporaryReply               []*tgbotapi.MessageConfig
	FlushTemporary               bool
	LeaveChat                    bool
	CallbackAlert                string // если не пусто — ответ на callback_query с показом алерта (popup)
	ApprovedMessageTrainingID    string // если не пусто — после отправки Reply в командный чат сохранить message_id как ApprovedMessageID для этой тренировки
	PendingReplyContext          *pendingreply.PendingReplyContext
	CoachNotificationOnDMFailure *pendingreply.PendingReplyContext // Уведомление тренерам при неудачной доставке личного сообщения
}

func NewAction() *Action {
	action := Action{
		Reply:                        make([]*tgbotapi.MessageConfig, 0),
		Edit:                         make([]*tgbotapi.EditMessageTextConfig, 0),
		Delete:                       make([]*tgbotapi.DeleteMessageConfig, 0),
		Pin:                          make([]*tgbotapi.MessageConfig, 0),
		TemporaryReply:               make([]*tgbotapi.MessageConfig, 0),
		FlushTemporary:                false,
		LeaveChat:                    false,
		PendingReplyContext:          nil,
		CoachNotificationOnDMFailure: nil,
	}

	return &action
}

func (a *Action) AddReply(msg *tgbotapi.MessageConfig) {
	a.Reply = append(a.Reply, msg)
}

func (a *Action) AddEdit(msg *tgbotapi.EditMessageTextConfig) {
	a.Edit = append(a.Edit, msg)
}

func (a *Action) AddDelete(msg *tgbotapi.DeleteMessageConfig) {
	a.Delete = append(a.Delete, msg)
}
func (a *Action) AddPin(msg *tgbotapi.MessageConfig) {
	a.Pin = append(a.Pin, msg)
}

func (a *Action) AddTemporaryReply(msg *tgbotapi.MessageConfig) {
	a.TemporaryReply = append(a.TemporaryReply, msg)
}

type Step func(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error)

type Scenario interface {
	ID() string
	GetStep(id int) (Step, error)
}

type BaseScenario struct {
	id    string
	steps []Step
}

func (s *BaseScenario) ID() string {
	return s.id
}

func (s *BaseScenario) GetStep(id int) (Step, error) {
	if len(s.steps) > id {
		return s.steps[id], nil
	}

	return nil, errors.New("step not found")
}

var globalUserTraining = make(map[int64]domain.Training)

func getGlobalUserTraining(userID int64) (*domain.Training, error) {
	if training, ok := globalUserTraining[userID]; ok {
		return &training, nil
	}

	return nil, errors.New("user training not found")
}

func saveGlobalUserTraining(userID int64, training domain.Training) {
	globalUserTraining[userID] = training
}

func clearGlobalUserTraining(userID int64) {
	delete(globalUserTraining, userID)
}

var globalUserPlayer = make(map[int64]domain.Player)

func getGlobalUserPlayer(userID int64) (*domain.Player, error) {
	if player, ok := globalUserPlayer[userID]; ok {
		return &player, nil
	}

	return nil, errors.New("user player not found")
}

func saveGlobalUserPlayer(userID int64, player domain.Player) {
	globalUserPlayer[userID] = player
}

func clearGlobalUserPlayer(userID int64) {
	delete(globalUserPlayer, userID)
}
