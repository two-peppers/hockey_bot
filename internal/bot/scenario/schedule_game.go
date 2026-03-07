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
	"unicode/utf8"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
)

const (
	gameCapacity int = 20
)

type ScheduleGame struct {
	BaseScenario
}

var _ Scenario = (*ScheduleGame)(nil)

func NewScheduleGame() *ScheduleGame {
	return &ScheduleGame{
		BaseScenario{
			id: "game",
			steps: []Step{
				askGameDate,
				checkGameDate,
				askGameTime,
				checkGameTime,
				askGameVenue,
				checkGameVenue,
				askGameDivision,
				checkGameDivision,
				askGameOpponent,
				checkGameOpponentAndCreateGame,
			},
		},
	}
}

func askGameDate(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	res := &Action{}

	teamID := data["team_id"]
	team, err := st.GetTeamByID(teamID)

	if err != nil {
		return nil, err
	}

	if team.ChatID == nil {
		errMsg := tgbotapi.NewMessage(chatID, "Перед созданием игры необходимо добавить бота в чат команды")
		res.AddReply(&errMsg)
		user_state.ClearUserState(userID, chatID)
		return res, nil
	}

	training := domain.Training{
		ID:     uuid.New().String(),
		TeamID: team.ID,
		Team:   *team,
		Type:   domain.TrainingMatch,
	}

	replyMsg := tgbotapi.NewMessage(chatID, "📅 Когда игра?")
	replyMsg.ReplyMarkup = keyboards.QuickDateReplyKeyboard()
	replyMsg.ParseMode = tgbotapi.ModeMarkdown

	deleteMsg := tgbotapi.NewDeleteMessage(chatID, messageID)

	res.AddReply(&replyMsg)
	res.AddDelete(&deleteMsg)

	user_state.SetUserState(userID, chatID, "game:1")

	saveGlobalUserTraining(userID, training)

	return res, nil
}

func checkGameDate(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
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

	user_state.SetUserState(userID, chatID, "game:2") // Спрашиваем про время тренировки
	saveGlobalUserTraining(userID, *training)

	return askGameTime(st, chatID, userID, messageID, data)
}

func askGameTime(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	res := NewAction()

	replyMsg := tgbotapi.NewMessage(chatID, "⏰ Во сколько?\nВведи время в формате ЧЧ:ММ")
	replyMsg.ReplyMarkup = tgbotapi.NewRemoveKeyboard(true)
	replyMsg.ParseMode = tgbotapi.ModeMarkdown

	res.AddReply(&replyMsg)

	user_state.SetUserState(userID, chatID, "game:3")

	return res, nil
}

func checkGameTime(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
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

	user_state.SetUserState(userID, chatID, "game:4") // Спрашиваем про арену

	return askGameVenue(st, chatID, userID, messageID, data)
}

func askGameVenue(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
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

	user_state.SetUserState(userID, chatID, "game:5")

	return res, nil
}

func checkGameVenue(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	training, err := getGlobalUserTraining(userID)
	if err != nil {
		return nil, err
	}

	venue := data["message"]
	training.Venue = venue

	saveGlobalUserTraining(userID, *training)
	user_state.SetUserState(userID, chatID, "game:6")

	return askGameDivision(st, chatID, userID, messageID, data)
}

func askGameDivision(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	res := NewAction()

	replyMsg := tgbotapi.NewMessage(chatID, "🏆 Какой дивизон?")
	replyMsg.ParseMode = tgbotapi.ModeMarkdown

	res.AddReply(&replyMsg)

	user_state.SetUserState(userID, chatID, "game:7")

	return res, nil
}

func checkGameDivision(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	training, err := getGlobalUserTraining(userID)
	if err != nil {
		return nil, err
	}

	division := data["message"]
	if utf8.RuneCountInString(division) > 32 {
		res := NewAction()

		replyMsg := tgbotapi.NewMessage(chatID, "❗ Слишком длинное назание диизона. Максимум 32 символа")
		replyMsg.ParseMode = tgbotapi.ModeMarkdown

		res.AddReply(&replyMsg)
		return res, nil
	}

	training.Division = division
	saveGlobalUserTraining(userID, *training)

	user_state.SetUserState(userID, chatID, "game:8")

	return askGameOpponent(st, chatID, userID, messageID, data)
}

func askGameOpponent(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	res := NewAction()

	replyMsg := tgbotapi.NewMessage(chatID, "🏒 С кем играем?")
	replyMsg.ParseMode = tgbotapi.ModeMarkdown

	res.AddReply(&replyMsg)
	user_state.SetUserState(userID, chatID, "game:9")

	return res, nil
}

func checkGameOpponentAndCreateGame(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	defer clearGlobalUserTraining(userID)
	defer user_state.ClearUserState(userID, chatID)

	res := NewAction()
	replyMsg := tgbotapi.NewMessage(chatID, "")
	replyMsg.ParseMode = tgbotapi.ModeMarkdown

	training, err := getGlobalUserTraining(userID)
	if err != nil {
		return nil, err
	}

	opponent := data["message"]
	if utf8.RuneCountInString(opponent) > 64 {
		replyMsg.Text = "🤔 Слишком длинное название команды. Попробуй еще раз"
		res.AddReply(&replyMsg)

		return res, nil
	}

	training.Opponent = opponent
	training.Capacity = gameCapacity

	err = st.CreateTraining(training)
	if err != nil {
		replyMsg.Text = "❌ Что-то пошло не так во время создания игры"
	} else {
		replyMsg.Text = "Игра запланирована!\n\n" +
			"📅 Дата: " + training.Date.Format("02.01.2006") + "\n" +
			"⏰ Время: " + training.Date.Format("15:04") + "\n" +
			"📍 Место: " + training.Venue + "\n" +
			"🏆 Дивизион: " + training.Division + "\n" +
			"💬 Соперник: " + training.Opponent

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
