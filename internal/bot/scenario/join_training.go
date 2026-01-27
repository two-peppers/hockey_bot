package scenario

import (
	"errors"
	"fmt"
	"hockey_bot/internal/bot/keyboards"
	"hockey_bot/internal/bot/user_state"
	"hockey_bot/internal/domain"
	"hockey_bot/internal/storage"
	"unicode/utf8"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
)

type JointTraining struct {
	BaseScenario
}

var _ Scenario = (*JointTraining)(nil)

var userTrainingMessageID = make(map[int64]int)
var userTrainingID = make(map[int64]string)

func NewJoinTraining() *JointTraining {
	return &JointTraining{
		BaseScenario{
			id: "go_tr",
			steps: []Step{
				askPlayerName,
				checkPlayerName,
				askPlayerPosition,
				checkPlayerPosition,
				askGoalkeeperCommercial,
				checkGoalkeeperCommercial,
				confirmPlayerJoin,
			},
		},
	}
}

func askPlayerName(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	trainingID := data.Get("id")

	training, err := st.GetTrainingByID(trainingID)

	if err != nil {
		return nil, err
	}

	player, err := st.GetPlayer(userID, training.TeamID)

	if err == nil && player != nil {
		data.Set("training_id", trainingID)
		return confirmPlayerJoin(st, chatID, userID, messageID, data)
	}

	userTrainingMessageID[userID] = messageID
	userTrainingID[userID] = trainingID
	res := NewAction()

	replyMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("👤 [%s](tg://user?id=%d), как твоя фамилия?", data.Get("username"), userID))
	replyMsg.ParseMode = tgbotapi.ModeMarkdown

	res.AddTemporaryReply(&replyMsg)

	user_state.SetUserState(userID, chatID, "go_tr:1")

	return res, nil
}

func checkPlayerName(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	user_state.AddToMessageBuffer(chatID, userID, messageID)
	player, err := getGlobalUserPlayer(userID)

	if err != nil || player == nil {
		player = &domain.Player{
			ID:         uuid.New().String(),
			TelegramID: userID,
		}
	}

	training, err := st.GetTrainingByID(userTrainingID[userID])

	if err != nil {
		return nil, err
	}

	fullName := data.Get("message")
	if utf8.RuneCountInString(fullName) > 32 {
		res := NewAction()
		replyMsg := tgbotapi.NewMessage(chatID, "🤔 Не похоже, что это твоя фамилия, очень длинная. Давай ещё раз попробуем")
		replyMsg.ParseMode = tgbotapi.ModeMarkdown
		replyMsg.ReplyToMessageID = messageID

		res.AddTemporaryReply(&replyMsg)

		return res, nil
	}

	player.FullName = fullName
	player.TeamID = training.TeamID
	saveGlobalUserPlayer(userID, *player)
	user_state.SetUserState(userID, chatID, "go_tr:2")

	return askPlayerPosition(st, chatID, userID, messageID, data)
}

func askPlayerPosition(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	player, err := getGlobalUserPlayer(userID)

	if err != nil {
		user_state.ClearUserState(userID, chatID)
		return nil, err
	}

	res := NewAction()

	replyMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("🎯 [%s](tg://user?id=%d), укажи свою позицию", player.FullName, userID))
	replyMsg.ParseMode = tgbotapi.ModeMarkdown
	replyMsg.ReplyMarkup = keyboards.PlayerPositionKeyboard("go_tr:3")

	res.AddTemporaryReply(&replyMsg)

	return res, nil
}

func checkPlayerPosition(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	player, err := getGlobalUserPlayer(userID)

	if err != nil {
		user_state.ClearUserState(userID, chatID)
		return nil, err
	}

	position := data.Get("position")
	player.Position = domain.PlayerPosition(position)

	trainingMessageID, ok := userTrainingMessageID[userID]
	if !ok {
		user_state.ClearUserState(userID, chatID)
		return nil, errors.New("trainingID not found")
	}

	if player.Position == domain.PositionGoalkeeper {
		saveGlobalUserPlayer(userID, *player)
		return askGoalkeeperCommercial(st, chatID, userID, trainingMessageID, data)
	}

	err = st.CreatePlayer(player)

	if err != nil {
		user_state.ClearUserState(userID, chatID)
		return nil, err
	}

	trainingID, ok := userTrainingID[userID]
	if !ok {
		user_state.ClearUserState(userID, chatID)
		return nil, errors.New("trainingID not found")
	}

	data.Set("training_id", trainingID)

	return confirmPlayerJoin(st, chatID, userID, trainingMessageID, data)
}

func askGoalkeeperCommercial(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	res := NewAction()

	player, err := getGlobalUserPlayer(userID)
	if err != nil {
		user_state.ClearUserState(userID, chatID)
		return nil, err
	}

	replyMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("🤑 [%s](tg://user?id=%d), за деньги играешь?", player.FullName, userID))
	replyMsg.ParseMode = tgbotapi.ModeMarkdown
	replyMsg.ReplyMarkup = keyboards.CommercialGoalkeeperKeyboard("go", "tr")

	res.AddTemporaryReply(&replyMsg)

	return res, nil
}

func checkGoalkeeperCommercial(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	player, err := getGlobalUserPlayer(userID)
	if err != nil {
		user_state.ClearUserState(userID, chatID)
		return nil, err
	}

	player.Commercial = data.Get("comm") == "true"

	trainingMessageID, ok := userTrainingMessageID[userID]
	if !ok {
		user_state.ClearUserState(userID, chatID)
		return nil, errors.New("trainingID not found")
	}

	err = st.CreatePlayer(player)
	if err != nil {
		user_state.ClearUserState(userID, chatID)
		return nil, err
	}

	trainingID, ok := userTrainingID[userID]
	if !ok {
		user_state.ClearUserState(userID, chatID)
		return nil, errors.New("trainingID not found")
	}

	data.Set("training_id", trainingID)

	return confirmPlayerJoin(st, chatID, userID, trainingMessageID, data)
}

func confirmPlayerJoin(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	res := NewAction()

	training, err := st.GetTrainingByID(data.Get("training_id"))
	if err != nil {
		user_state.ClearUserState(userID, chatID)
		return nil, err
	}

	player, err := st.GetPlayer(userID, training.TeamID)

	if err != nil {
		user_state.ClearUserState(userID, chatID)
		return nil, err
	}

	training.AddPlayer(player)
	st.SaveTraining(training)

	keyboard := keyboards.TakePartKeyboard("tr", training.ID)
	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, training.String())
	editMsg.ParseMode = tgbotapi.ModeMarkdown
	editMsg.ReplyMarkup = &keyboard

	res.AddEdit(&editMsg)
	res.FlushTemporary = true

	delete(userTrainingMessageID, userID)
	delete(userTrainingID, userID)
	user_state.ClearUserState(userID, chatID)
	clearGlobalUserPlayer(userID)

	return res, nil
}
