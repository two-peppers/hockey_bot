package scenario

import (
	"fmt"
	"hockey_bot/internal/bot/keyboards"
	"hockey_bot/internal/bot/user_state"
	"hockey_bot/internal/domain"
	"hockey_bot/internal/storage"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
)

type ScheduleTraining struct {
	BaseScenario
}

var _ Scenario = (*ScheduleTraining)(nil)

func NewScheduleTraining() *ScheduleTraining {
	return &ScheduleTraining{
		BaseScenario{
			id: "train",
			steps: []Step{
				askTrainingDate,
				checkTrainingDate,
				askExactTrainingDate,
				checkExactTrainingDate,
				askTrainingTime,
				checkTrainingTime,
				askTrainingVenue,
				checkTrainingVenue,
				askTrainingFormat,
				checkTrainingFormat,
				askTrainingCapacity,
				checkTrainingCapacity,
				askTrainingBottomMessage,
				checkTrainingBottomMessageAndCreateTraining,
			},
		},
	}
}

func askTrainingDate(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	res := &Action{}

	teamID := data["team_id"]
	team, err := st.GetTeamByID(teamID)

	if err != nil {
		return nil, err
	}

	if team.ChatID == nil {
		errMsg := tgbotapi.NewMessage(chatID, "Перед созданием тренировки необходимо добавить бота в чат команды")
		res.AddReply(&errMsg)
		user_state.ClearUserState(userID, chatID)
		return res, nil
	}

	training := domain.Training{
		ID:     uuid.New().String(),
		TeamID: team.ID,
		Team:   *team,
	}

	replyMsg := tgbotapi.NewMessage(chatID, "📅 Когда тренировка?")
	replyMsg.ReplyMarkup = keyboards.QuickDateReplyKeyboard()
	replyMsg.ParseMode = tgbotapi.ModeMarkdown

	deleteMsg := tgbotapi.NewDeleteMessage(chatID, messageID)

	res.AddReply(&replyMsg)
	res.AddDelete(&deleteMsg)

	user_state.SetUserState(userID, chatID, "train:1")

	saveGlobalUserTraining(userID, training)

	return res, nil
}

func checkTrainingDate(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	training, err := getGlobalUserTraining(userID)

	if err != nil {
		return nil, err
	}

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	message := data["message"]

	switch message {
	case "Сегодня":
		training.Date = today
		break
	case "Завтра":
		training.Date = today.Add(24 * time.Hour)
		break
	case "Послезавтра":
		training.Date = today.Add(2 * 24 * time.Hour)
		break
	case "+3 дня":
		training.Date = today.Add(3 * 24 * time.Hour)
		break
	default:
		var replyMsgText string
		date, err := time.Parse("02.01.2006", message)

		if err != nil {
			replyMsgText = "Неверный формат даты. Введи дату в формате ДД.ММ.ГГГГ"
			replyMsg := tgbotapi.NewMessage(chatID, replyMsgText)
			replyMsg.ParseMode = tgbotapi.ModeMarkdown

			res := NewAction()
			res.AddReply(&replyMsg)

			return res, nil
		}

		training.Date = date
	}

	user_state.SetUserState(userID, chatID, "train:4") // Спрашиваем про время тренировки
	saveGlobalUserTraining(userID, *training)

	return askTrainingTime(st, chatID, userID, messageID, data)
}

// Deprecated: точная дата спрашивается вместе с клавиатурой
func askExactTrainingDate(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	res := NewAction()
	replyMsg := tgbotapi.NewMessage(chatID, "Введи дату в формате ДД.ММ.ГГГГ")

	res.AddReply(&replyMsg)

	return res, nil
}

// Deprecated: см. выше
func checkExactTrainingDate(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	res := &Action{}

	message := data["message"]
	date, err := time.Parse("02.01.2006", message)

	replyMsg := tgbotapi.NewMessage(chatID, "")
	replyMsg.ParseMode = tgbotapi.ModeMarkdown

	if err != nil {
		user_state.SetUserState(userID, chatID, "train:3")
		replyMsg.Text = "Некоррктный формат даты. Попробуй еще раз: ДД.ММ.ГГГГ"
		res.AddReply(&replyMsg)

		return res, nil
	}

	training, err := getGlobalUserTraining(userID)

	if err != nil {
		user_state.SetUserState(userID, chatID, "train:0")
		return askExactTrainingDate(st, chatID, userID, messageID, data)
	}

	training.Date = date
	saveGlobalUserTraining(userID, *training)
	user_state.SetUserState(userID, chatID, "train:5") // Переходим к времени тренировки

	return askTrainingTime(st, chatID, userID, messageID, data)
}

func askTrainingTime(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	res := NewAction()

	replyMsg := tgbotapi.NewMessage(chatID, "⏰ Во сколько?\nВведи время в формате ЧЧ:ММ")
	replyMsg.ReplyMarkup = tgbotapi.NewRemoveKeyboard(true)
	replyMsg.ParseMode = tgbotapi.ModeMarkdown

	res.AddReply(&replyMsg)

	user_state.SetUserState(userID, chatID, "train:5")

	return res, nil
}

func checkTrainingTime(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	res := &Action{}

	text := data["message"]
	replyMsg := tgbotapi.NewMessage(chatID, "")
	replyMsg.ParseMode = tgbotapi.ModeMarkdown

	parts := strings.Split(text, ":")
	if len(parts) != 2 {
		replyMsg.Text = "❌ Неверный формат времени. Введи время в формате *ЧЧ:ММ*."
		res.AddReply(&replyMsg)
		return res, nil
	}

	hours, _ := strconv.Atoi(parts[0])
	minutes, _ := strconv.Atoi(parts[1])

	if hours < 0 || hours > 23 || minutes < 0 || minutes > 59 {
		replyMsg.Text = "❌ Неверный формат времени. Введи время в формате *ЧЧ:ММ*."
		res.AddReply(&replyMsg)
		return res, nil
	}

	training, err := getGlobalUserTraining(userID)

	if err != nil {
		return nil, err
	}

	training.Date = training.Date.Add(time.Duration(hours) * time.Hour).Add(time.Duration(minutes) * time.Minute)
	saveGlobalUserTraining(userID, *training)

	user_state.SetUserState(userID, chatID, "train:6") // Спрашиваем про арену

	return askTrainingVenue(st, chatID, userID, messageID, data)
}

func askTrainingVenue(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	res := NewAction()

	training, err := getGlobalUserTraining(userID)

	if err != nil {
		return nil, err
	}

	replyMsg := tgbotapi.NewMessage(chatID, "📍 На какой арене?")
	replyMsg.ParseMode = tgbotapi.ModeMarkdown

	venues := st.GetRecentVenues(training.TeamID)
	if len(venues) > 0 {
		replyMsg.ReplyMarkup = keyboards.VenueKeyboard(venues)
	}

	res.AddReply(&replyMsg)

	user_state.SetUserState(userID, chatID, "train:7")

	return res, nil
}

func checkTrainingVenue(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	training, err := getGlobalUserTraining(userID)

	if err != nil {
		return nil, err
	}

	venue := data["message"]
	training.Venue = venue

	saveGlobalUserTraining(userID, *training)

	return askTrainingFormat(st, chatID, userID, messageID, data)
}

func askTrainingFormat(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	res := &Action{}

	replyMsg := tgbotapi.NewMessage(chatID, "Формат тренировки")
	replyMsg.ParseMode = tgbotapi.ModeMarkdown
	replyMsg.ReplyMarkup = keyboards.TrainingTypeKeyboard()

	res.AddReply(&replyMsg)

	user_state.SetUserState(userID, chatID, "train:9")

	return res, nil
}

func checkTrainingFormat(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	training, _ := getGlobalUserTraining(userID)

	trainingType := data["type"]
	training.Type = domain.TrainingType(trainingType)

	saveGlobalUserTraining(userID, *training)

	action := NewAction()

	deleteMsg := tgbotapi.NewDeleteMessage(chatID, messageID)
	action.AddDelete(&deleteMsg)

	replyMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Формат тренировки: %s", training.Type.ToString()))
	action.AddReply(&replyMsg)

	newAction, _ := askTrainingCapacity(st, chatID, userID, messageID, data)

	action.AddReply(newAction.Reply[0])

	return action, nil
}

func askTrainingCapacity(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	res := &Action{}

	replyMsg := tgbotapi.NewMessage(chatID, "👥 Сколько игроков максимум?")
	replyMsg.ParseMode = tgbotapi.ModeMarkdown
	replyMsg.ReplyMarkup = keyboards.TrainingCapacityKeyboard()

	res.AddReply(&replyMsg)

	user_state.SetUserState(userID, chatID, "train:11")

	return res, nil
}

func checkTrainingCapacity(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	training, err := getGlobalUserTraining(userID)

	if err != nil {
		return nil, err
	}

	capacity, err := strconv.Atoi(data["message"])

	if err != nil {
		replyMsg := tgbotapi.NewMessage(chatID, "❌ Количество может быть только числом. Попробуй еще раз.")
		replyMsg.ParseMode = tgbotapi.ModeMarkdown

		res := NewAction()
		res.AddReply(&replyMsg)

		return res, nil
	}

	training.Capacity = capacity
	saveGlobalUserTraining(userID, *training)

	action, err := askTrainingBottomMessage(st, chatID, userID, messageID, data)

	return action, err
}

func askTrainingBottomMessage(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	res := NewAction()

	replyMsg := tgbotapi.NewMessage(chatID, "Ок, понял! Последний вопрос")
	replyMsg.ParseMode = tgbotapi.ModeMarkdown
	replyMsg.ReplyMarkup = tgbotapi.NewRemoveKeyboard(true)

	res.AddReply(&replyMsg)

	replyMsg2 := tgbotapi.NewMessage(chatID, "💬 Сообщение внизу")
	replyMsg2.ParseMode = tgbotapi.ModeMarkdown
	replyMsg2.ReplyMarkup = keyboards.TrainingNoMessageKeyboard()
	res.AddReply(&replyMsg2)

	user_state.SetUserState(userID, chatID, "train:13")

	return res, nil
}

func checkTrainingBottomMessageAndCreateTraining(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	defer clearGlobalUserTraining(userID)
	defer user_state.ClearUserState(userID, chatID)

	training, err := getGlobalUserTraining(userID)
	if err != nil {
		return nil, err
	}

	res := NewAction()

	message := data["message"]

	if len(message) == 0 {
		deleteMsg := tgbotapi.NewDeleteMessage(chatID, messageID)
		res.AddDelete(&deleteMsg)
	}

	training.Message = message

	replyMsg := tgbotapi.NewMessage(chatID, "")
	replyMsg.ParseMode = tgbotapi.ModeMarkdown

	err = st.CreateTraining(training)
	if err != nil {
		replyMsg.Text = "❌ Что-то пошло не так во время создания тренировки"
	} else {
		replyMsg.Text = "Тренировка создана!\n\n" +
			"📅 Дата: " + training.Date.Format("02.01.2006") + "\n" +
			"⏰ Время: " + training.Date.Format("15:04") + "\n" +
			"📍 Место: " + training.Venue + "\n" +
			"👥 Участников: " + strconv.Itoa(training.Capacity) + "\n" +
			"💬 Сообщение: " + training.Message
		replyMsg.ParseMode = tgbotapi.ModeMarkdown
		replyMsg.ReplyMarkup = tgbotapi.NewRemoveKeyboard(true)

		chatMessage := tgbotapi.NewMessage(*training.Team.ChatID, fmt.Sprintf("%s", training))
		chatMessage.ParseMode = tgbotapi.ModeMarkdown
		chatMessage.ReplyMarkup = keyboards.TakePartKeyboard("tr", training.ID)
		res.AddPin(&chatMessage)
		res.TeamMessageTrainingID = training.ID
	}

	res.AddReply(&replyMsg)

	return res, nil
}
