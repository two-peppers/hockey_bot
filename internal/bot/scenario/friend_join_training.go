package scenario

import (
	"errors"
	"fmt"
	"hockey_bot/internal/bot/keyboards"
	"hockey_bot/internal/bot/user_state"
	"hockey_bot/internal/domain"
	"hockey_bot/internal/storage"
	"strconv"
	"unicode/utf8"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
)

type FriendJoinTraining struct {
	BaseScenario
}

var _ Scenario = (*FriendJoinTraining)(nil)

var userFriendTrainingID = make(map[sessionKey]string)
var userContextMessage = make(map[sessionKey]int)

func NewFriendJoinTraining() *FriendJoinTraining {
	return &FriendJoinTraining{
		BaseScenario{
			id: "frgo_tr",
			steps: []Step{
				askFriendName,
				checkFriendName,
				askFriendPosition,
				checkFriendPosition,
				askFriendGoalkeeperCommercial,
				checkFriendGoalkeeperCommercial,
				confirmFriendJoinTraining,
				showFriendsPage,
			},
		},
	}
}

func askFriendName(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	trainingID := data["id"]
	training, err := getTraining(st, trainingID)
	if err != nil {
		return nil, err
	}

	player, err := st.GetPlayer(userID, training.TeamID)

	if err != nil || player == nil {
		return nil, errors.New("player not found")
	}

	userFriendTrainingID[sessionKey{userID, chatID}] = trainingID
	userContextMessage[sessionKey{userID, chatID}] = messageID

	friends, err := st.GetPlayersByTelegramID(training.TeamID, userID)
	if err != nil {
		return nil, err
	}
	if len(friends) == 0 {
		return nil, ErrNotFound
	}
	friends = friends[1:]

	res := NewAction()

	replyMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("👤 [%s](tg://user?id=%d), как фамилия друга?", data.Get("username"), userID))
	replyMsg.ParseMode = tgbotapi.ModeMarkdown

	if len(friends) > 0 {
		replyMsg.ReplyMarkup = keyboards.PaginatedFriendsKeyboard("frgo", "tr", 6, 7, 0, friends)
		replyMsg.Text += "\n\n_(можешь выбрать из списка ниже)_"
	}

	res.AddTemporaryReply(&replyMsg)

	user_state.SetUserState(userID, chatID, "frgo_tr:1")

	return res, nil
}

func showFriendsPage(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	training, err := getTraining(st, userFriendTrainingID[sessionKey{userID, chatID}])
	if err != nil || training == nil {
		return nil, errors.New("training not found")
	}
	friends, err := st.GetPlayersByTelegramID(training.TeamID, userID)
	if err != nil || len(friends) < 2 {
		return nil, errors.New("friends not found")
	}
	friends = friends[1:]
	page, _ := strconv.Atoi(data.Get("p"))
	text := fmt.Sprintf("👤 [%s](tg://user?id=%d), как фамилия друга?\n\n_(можешь выбрать из списка ниже)_", data.Get("username"), userID)
	keyboard := keyboards.PaginatedFriendsKeyboard("frgo", "tr", 6, 7, page, friends)
	edit := tgbotapi.NewEditMessageText(chatID, messageID, text)
	edit.ParseMode = tgbotapi.ModeMarkdown
	edit.ReplyMarkup = &keyboard
	res := NewAction()
	res.AddEdit(&edit)
	user_state.SetUserState(userID, chatID, "frgo_tr:1")
	return res, nil
}

func checkFriendName(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	user_state.AddToMessageBuffer(chatID, userID, messageID)
	player, err := getGlobalUserPlayer(userID, chatID)

	if err != nil || player == nil {
		player = &domain.Player{
			ID:         uuid.New().String(),
			TelegramID: userID,
		}
	}

	training, err := getTraining(st, userFriendTrainingID[sessionKey{userID, chatID}])

	if err != nil {
		return nil, err
	}

	fullName := data.Get("message")
	if utf8.RuneCountInString(fullName) > 32 {
		res := NewAction()
		replyMsg := tgbotapi.NewMessage(chatID, "🤔 Не похоже, что это фамилия, очень длинная. Давай ещё раз попробуем")
		replyMsg.ParseMode = tgbotapi.ModeMarkdown
		replyMsg.ReplyToMessageID = messageID

		res.AddTemporaryReply(&replyMsg)

		return res, nil
	}

	player.FullName = fullName
	player.TeamID = training.TeamID
	saveGlobalUserPlayer(userID, chatID, *player)
	user_state.SetUserState(userID, chatID, "frgo_tr:2")

	return askFriendPosition(st, chatID, userID, messageID, data)
}

func askFriendPosition(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	player, err := getGlobalUserPlayer(userID, chatID)

	if err != nil {
		user_state.ClearUserState(userID, chatID)
		return nil, err
	}

	res := NewAction()

	replyMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("🎯 На какой позиции играет %s?", player.FullName))
	replyMsg.ParseMode = tgbotapi.ModeMarkdown
	replyMsg.ReplyMarkup = keyboards.PlayerPositionKeyboard("frgo_tr:3")

	res.AddTemporaryReply(&replyMsg)

	return res, nil
}

func checkFriendPosition(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	player, err := getGlobalUserPlayer(userID, chatID)

	if err != nil {
		user_state.ClearUserState(userID, chatID)
		return nil, err
	}

	position := data.Get("position")
	player.Position = domain.PlayerPosition(position)

	trainingMessageID, ok := userContextMessage[sessionKey{userID, chatID}]
	if !ok {
		user_state.ClearUserState(userID, chatID)
		return nil, errors.New("trainingID not found")
	}

	if player.Position == domain.PositionGoalkeeper {
		saveGlobalUserPlayer(userID, chatID, *player)
		return askFriendGoalkeeperCommercial(st, chatID, userID, trainingMessageID, data)
	}

	err = st.CreatePlayer(player)

	if err != nil {

		user_state.ClearUserState(userID, chatID)
		return nil, err
	}

	trainingID, ok := userFriendTrainingID[sessionKey{userID, chatID}]
	if !ok {
		user_state.ClearUserState(userID, chatID)
		return nil, errors.New("trainingID not found")
	}

	data.Set("training_id", trainingID)
	data.Set("id", player.ID)

	return confirmFriendJoinTraining(st, chatID, userID, trainingMessageID, data)
}

func askFriendGoalkeeperCommercial(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	res := NewAction()

	player, err := getGlobalUserPlayer(userID, chatID)
	if err != nil {
		user_state.ClearUserState(userID, chatID)
		return nil, err
	}

	replyMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("🤑 %s играет за деньги?", player.FullName))
	replyMsg.ParseMode = tgbotapi.ModeMarkdown
	replyMsg.ReplyMarkup = keyboards.CommercialGoalkeeperKeyboard("frgo", "tr")

	res.AddTemporaryReply(&replyMsg)

	return res, nil
}

func checkFriendGoalkeeperCommercial(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	player, err := getGlobalUserPlayer(userID, chatID)
	if err != nil {
		user_state.ClearUserState(userID, chatID)
		return nil, err
	}

	player.Commercial = data.Get("comm") == "true"

	trainingMessageID, ok := userContextMessage[sessionKey{userID, chatID}]
	if !ok {
		user_state.ClearUserState(userID, chatID)
		return nil, errors.New("trainingID not found")
	}

	err = st.CreatePlayer(player)
	if err != nil {
		user_state.ClearUserState(userID, chatID)
		return nil, err
	}

	trainingID, ok := userFriendTrainingID[sessionKey{userID, chatID}]
	if !ok {
		user_state.ClearUserState(userID, chatID)
		return nil, errors.New("trainingID not found")
	}

	data.Set("training_id", trainingID)
	data.Set("id", player.ID)

	return confirmFriendJoinTraining(st, chatID, userID, trainingMessageID, data)
}

func confirmFriendJoinTraining(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	var exist bool
	res := NewAction()

	trainingID := data.Get("training_id")

	if len(trainingID) == 0 {
		trainingID, exist = userFriendTrainingID[sessionKey{userID, chatID}]
		if !exist {
			return nil, errors.New("trainingID not set")
		}
	}

	training, err := getTraining(st, trainingID)
	if err != nil {
		user_state.ClearUserState(userID, chatID)
		return nil, err
	}

	player, err := getOwnedPlayer(st, data.Get("id"), training.TeamID, userID)

	if err != nil {
		user_state.ClearUserState(userID, chatID)
		return nil, err
	}

	if player == nil {
		return nil, ErrNotFound
	}
	messageID, exist = userContextMessage[sessionKey{userID, chatID}]
	if !exist {
		return nil, errors.New("messageID not set")
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

	delete(userContextMessage, sessionKey{userID, chatID})
	delete(userFriendTrainingID, sessionKey{userID, chatID})
	user_state.ClearUserState(userID, chatID)
	clearGlobalUserPlayer(userID, chatID)

	return res, nil
}
