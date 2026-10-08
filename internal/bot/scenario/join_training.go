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

var userTrainingMessageID = make(map[sessionKey]int)
var userTrainingID = make(map[sessionKey]string)

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

	training, err := getTraining(st, trainingID)

	if err != nil {
		return nil, err
	}

	player, err := st.GetPlayer(userID, training.TeamID)

	if err != nil {
		return nil, err
	}
	if player != nil {
		data.Set("training_id", trainingID)
		return confirmPlayerJoin(st, chatID, userID, messageID, data)
	}

	userTrainingMessageID[sessionKey{userID, chatID}] = messageID
	userTrainingID[sessionKey{userID, chatID}] = trainingID
	res := NewAction()

	replyMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("👤 [%s](tg://user?id=%d), как твоя фамилия?", data.Get("username"), userID))
	replyMsg.ParseMode = tgbotapi.ModeMarkdown

	res.AddTemporaryReply(&replyMsg)

	user_state.SetUserState(userID, chatID, "go_tr:1")

	return res, nil
}

func checkPlayerName(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	user_state.AddToMessageBuffer(chatID, userID, messageID)
	player, err := getGlobalUserPlayer(userID, chatID)

	if err != nil || player == nil {
		player = &domain.Player{
			ID:         uuid.New().String(),
			TelegramID: userID,
		}
	}

	training, err := getTraining(st, userTrainingID[sessionKey{userID, chatID}])

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
	saveGlobalUserPlayer(userID, chatID, *player)
	user_state.SetUserState(userID, chatID, "go_tr:2")

	return askPlayerPosition(st, chatID, userID, messageID, data)
}

func askPlayerPosition(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	player, err := getGlobalUserPlayer(userID, chatID)

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
	player, err := getGlobalUserPlayer(userID, chatID)

	if err != nil {
		user_state.ClearUserState(userID, chatID)
		return nil, err
	}

	position := data.Get("position")
	player.Position = domain.PlayerPosition(position)

	trainingMessageID, ok := userTrainingMessageID[sessionKey{userID, chatID}]
	if !ok {
		user_state.ClearUserState(userID, chatID)
		return nil, errors.New("trainingID not found")
	}

	if player.Position == domain.PositionGoalkeeper {
		saveGlobalUserPlayer(userID, chatID, *player)
		return askGoalkeeperCommercial(st, chatID, userID, trainingMessageID, data)
	}

	err = st.CreatePlayer(player)

	if err != nil {
		user_state.ClearUserState(userID, chatID)
		return nil, err
	}

	trainingID, ok := userTrainingID[sessionKey{userID, chatID}]
	if !ok {
		user_state.ClearUserState(userID, chatID)
		return nil, errors.New("trainingID not found")
	}

	data.Set("training_id", trainingID)

	return confirmPlayerJoin(st, chatID, userID, trainingMessageID, data)
}

func askGoalkeeperCommercial(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	res := NewAction()

	player, err := getGlobalUserPlayer(userID, chatID)
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
	player, err := getGlobalUserPlayer(userID, chatID)
	if err != nil {
		user_state.ClearUserState(userID, chatID)
		return nil, err
	}

	player.Commercial = data.Get("comm") == "true"

	trainingMessageID, ok := userTrainingMessageID[sessionKey{userID, chatID}]
	if !ok {
		user_state.ClearUserState(userID, chatID)
		return nil, errors.New("trainingID not found")
	}

	err = st.CreatePlayer(player)
	if err != nil {
		user_state.ClearUserState(userID, chatID)
		return nil, err
	}

	trainingID, ok := userTrainingID[sessionKey{userID, chatID}]
	if !ok {
		user_state.ClearUserState(userID, chatID)
		return nil, errors.New("trainingID not found")
	}

	data.Set("training_id", trainingID)

	return confirmPlayerJoin(st, chatID, userID, trainingMessageID, data)
}

func confirmPlayerJoin(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	res := NewAction()

	training, err := getTraining(st, data.Get("training_id"))
	if err != nil {
		user_state.ClearUserState(userID, chatID)
		return nil, err
	}

	player, err := st.GetPlayer(userID, training.TeamID)

	if err != nil {
		user_state.ClearUserState(userID, chatID)
		return nil, err
	}

	if player == nil {
		return nil, ErrNotFound
	}
	if err := st.AddTrainingPlayer(training.ID, player.ID); err != nil {
		return nil, err
	}
	training, err = getTraining(st, training.ID)
	if err != nil {
		return nil, err
	}

	keyboard := keyboards.TakePartKeyboard("tr", training.ID)
	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, training.String())
	editMsg.ParseMode = tgbotapi.ModeMarkdown
	editMsg.ReplyMarkup = &keyboard

	res.RefreshTrainingID = training.ID
	res.AddEdit(&editMsg)
	if chatID > 0 && training.Team.ChatID != nil && *training.Team.ChatID != 0 && training.TeamMessageID != 0 {
		teamEdit := tgbotapi.NewEditMessageText(*training.Team.ChatID, training.TeamMessageID, training.String())
		teamEdit.ParseMode = tgbotapi.ModeMarkdown
		teamEdit.ReplyMarkup = &keyboard
		res.AddEdit(&teamEdit)
	}
	res.FlushTemporary = true

	delete(userTrainingMessageID, sessionKey{userID, chatID})
	delete(userTrainingID, sessionKey{userID, chatID})
	user_state.ClearUserState(userID, chatID)
	clearGlobalUserPlayer(userID, chatID)

	return res, nil
}
