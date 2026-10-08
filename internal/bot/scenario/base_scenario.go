package scenario

import (
	"encoding/json"
	"errors"
	"fmt"
	"hockey_bot/internal/domain"
	"hockey_bot/internal/pendingreply"
	"hockey_bot/internal/storage"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Data map[string]string

// Старые кнопки пагинации передавали номер страницы как JSON-число.
func (d *Data) UnmarshalJSON(raw []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	if fields == nil {
		return errors.New("expected callback object")
	}
	result := make(Data, len(fields))
	for key, value := range fields {
		var str string
		if err := json.Unmarshal(value, &str); err == nil && string(value) != "null" {
			result[key] = str
			continue
		}
		var number json.Number
		if err := json.Unmarshal(value, &number); err != nil || number == "" {
			return fmt.Errorf("invalid callback field %q", key)
		}
		result[key] = number.String()
	}
	*d = result
	return nil
}

func (d *Data) Get(key string) string {
	if val, ok := (*d)[key]; ok {
		return val
	}

	return ""
}

func (d *Data) Set(key, value string) {
	if *d == nil {
		*d = make(Data)
	}
	(*d)[key] = value
}

type Action struct {
	RefreshTrainingID            string // перечитать состав перед обновлением сообщений
	Reply                        []*tgbotapi.MessageConfig
	Edit                         []*tgbotapi.EditMessageTextConfig
	Delete                       []*tgbotapi.DeleteMessageConfig
	Pin                          []*tgbotapi.MessageConfig
	TemporaryReply               []*tgbotapi.MessageConfig
	FlushTemporary               bool
	LeaveChat                    bool
	CallbackAlert                string // если не пусто — ответ на callback_query с показом алерта (popup)
	TeamMessageTrainingID        string // если не пусто — после отправки Pin в командный чат сохранить message_id как TeamMessageID для этой тренировки
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
		FlushTemporary:               false,
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
	if id >= 0 && len(s.steps) > id {
		return func(st storage.Storage, chatID, userID int64, messageID int, data Data) (*Action, error) {
			// Черновики и переходы сценариев меняются атомарно. Сетевые
			// запросы к Telegram выполняются роутером уже без этой блокировки.
			scenarioMu.Lock()
			defer scenarioMu.Unlock()
			return s.steps[id](st, chatID, userID, messageID, data)
		}, nil
	}

	return nil, errors.New("step not found")
}

var scenarioMu sync.Mutex

type sessionKey struct{ userID, chatID int64 }

var globalUserTraining = make(map[sessionKey]domain.Training)

func getGlobalUserTraining(userID, chatID int64) (*domain.Training, error) {
	if training, ok := globalUserTraining[sessionKey{userID, chatID}]; ok {
		return &training, nil
	}

	return nil, errors.New("user training not found")
}

func saveGlobalUserTraining(userID, chatID int64, training domain.Training) {
	globalUserTraining[sessionKey{userID, chatID}] = training
}

func clearGlobalUserTraining(userID, chatID int64) {
	delete(globalUserTraining, sessionKey{userID, chatID})
}

var globalUserPlayer = make(map[sessionKey]domain.Player)

func getGlobalUserPlayer(userID, chatID int64) (*domain.Player, error) {
	if player, ok := globalUserPlayer[sessionKey{userID, chatID}]; ok {
		return &player, nil
	}

	return nil, errors.New("user player not found")
}

func saveGlobalUserPlayer(userID, chatID int64, player domain.Player) {
	globalUserPlayer[sessionKey{userID, chatID}] = player
}

func clearGlobalUserPlayer(userID, chatID int64) {
	delete(globalUserPlayer, sessionKey{userID, chatID})
}
