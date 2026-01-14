package scenario

import (
	"errors"
	"fmt"
	"hockey_bot/internal/bot/keyboards"
	"hockey_bot/internal/bot/user_state"
	"hockey_bot/internal/domain"
	"hockey_bot/internal/storage"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
)

type FriendJoinTraining struct {
	BaseScenario
}

var _ Scenario = (*FriendJoinTraining)(nil)

var userFriendTrainingID = make(map[int64]string)
var userContextMessage = make(map[int64]int)

func NewFriendJoinTraining() *FriendJoinTraining {
	return &FriendJoinTraining{
		BaseScenario{
			id: "frgo_tr",
			steps: []Step{
				askFriendName,
				checkFriendName,
				askFriendPosition,
				checkFriendPosition,
				confirmFriendJoinTraining,
			},
		},
	}
}

func askFriendName(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	trainingID := data["id"]
	training, _ := st.GetTrainingByID(trainingID)

	player, err := st.GetPlayer(userID, training.TeamID)

	if err != nil || player == nil {
		return nil, errors.New("player not found")
	}

	userFriendTrainingID[userID] = trainingID
	userContextMessage[userID] = messageID

	friends, _ := st.GetPlayersByTelegramID(training.TeamID, userID)
	friends = friends[1:]

	res := NewAction()

	replyMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("👤 [%s](tg://user?id=%d), как фамилия друга?", data.Get("username"), userID))
	replyMsg.ParseMode = tgbotapi.ModeMarkdown

	if len(friends) > 0 {
		replyMsg.ReplyMarkup = keyboards.FriendsKeyboard("frgo", "tr", 4, friends)
		replyMsg.Text += "\n\n_(можешь выбрать из списка ниже)_"
	}

	res.AddTemporaryReply(&replyMsg)

	user_state.SetUserState(userID, chatID, "frgo_tr:1")

	return res, nil
}

func checkFriendName(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	player, err := getGlobalUserPlayer(userID)

	if err != nil || player == nil {
		player = &domain.Player{
			ID:         uuid.New().String(),
			TelegramID: userID,
		}
	}

	training, err := st.GetTrainingByID(userFriendTrainingID[userID])

	if err != nil {
		return nil, err
	}

	player.FullName = data.Get("message")
	player.TeamID = training.TeamID
	saveGlobalUserPlayer(userID, *player)
	user_state.AddToMessageBuffer(chatID, userID, messageID)
	user_state.SetUserState(userID, chatID, "frgo_tr:2")

	return askFriendPosition(st, chatID, userID, messageID, data)
}

func askFriendPosition(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	player, err := getGlobalUserPlayer(userID)

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
	player, err := getGlobalUserPlayer(userID)

	if err != nil {
		user_state.ClearUserState(userID, chatID)
		return nil, err
	}

	position := data.Get("position")
	player.Position = domain.PlayerPosition(position)

	err = st.CreatePlayer(player)

	if err != nil {

		user_state.ClearUserState(userID, chatID)
		return nil, err
	}

	trainingMessageID, ok := userContextMessage[userID]
	if !ok {
		user_state.ClearUserState(userID, chatID)
		return nil, errors.New("trainingID not found")
	}

	trainingID, ok := userFriendTrainingID[userID]
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
		trainingID, exist = userFriendTrainingID[userID]
		if !exist {
			return nil, errors.New("trainingID not set")
		}
	}

	training, err := st.GetTrainingByID(trainingID)
	if err != nil {
		user_state.ClearUserState(userID, chatID)
		return nil, err
	}

	player, err := st.GetPlayerByID(data.Get("id"))

	if err != nil {
		user_state.ClearUserState(userID, chatID)
		return nil, err
	}

	training.AddPlayer(player)
	st.SaveTraining(training)

	messageID, exist = userContextMessage[userID]

	if !exist {
		return nil, errors.New("messageID not set")
	}

	keyboard := keyboards.TakePartKeyboard("tr", training.ID)
	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, training.String())
	editMsg.ParseMode = tgbotapi.ModeMarkdown
	editMsg.ReplyMarkup = &keyboard

	res.AddEdit(&editMsg)
	res.FlushTemporary = true

	delete(userContextMessage, userID)
	delete(userFriendTrainingID, userID)
	user_state.ClearUserState(userID, chatID)
	clearGlobalUserPlayer(userID)

	return res, nil
}
