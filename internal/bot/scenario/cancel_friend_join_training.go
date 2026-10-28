package scenario

import (
	"errors"
	"fmt"
	"hockey_bot/internal/bot/keyboards"
	"hockey_bot/internal/bot/user_state"
	"hockey_bot/internal/domain"
	"hockey_bot/internal/storage"
	"strconv"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type CancelFriendJoinTraining struct {
	BaseScenario
}

var _ Scenario = (*CancelFriendJoinTraining)(nil)

var userCancelFriendTrainingID = make(map[sessionKey]string)
var userCancelContextMessage = make(map[sessionKey]int)

func NewCancelFriendJoinTraining() *CancelFriendJoinTraining {
	return &CancelFriendJoinTraining{
		BaseScenario{
			id: "frnogo",
			steps: []Step{
				askFriendNameForCancel,
				confirmFriendCancelTraining,
				showCancelFriendsPage,
			},
		},
	}
}

func askFriendNameForCancel(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	userCancelContextMessage[sessionKey{userID, chatID}] = messageID

	trainingID := data["id"]
	training, err := getTraining(st, trainingID)
	if err != nil {
		return nil, err
	}

	userCancelFriendTrainingID[sessionKey{userID, chatID}] = trainingID

	friendsInTraining, err := getFriendsInTraining(st, training, userID)
	if err != nil || len(friendsInTraining) == 0 {
		return nil, errors.New("no friends in training")
	}

	replyMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("[%s](tg://user?id=%d), какой друг не сможет пойти?", data.Get("username"), userID))
	replyMsg.ParseMode = tgbotapi.ModeMarkdown
	replyMsg.ReplyMarkup = keyboards.PaginatedFriendsKeyboard("frnogo", "tr", 1, 2, 0, friendsInTraining)

	user_state.SetUserState(userID, chatID, "frnogo_tr:1")

	res := NewAction()
	res.AddTemporaryReply(&replyMsg)

	return res, nil
}

func showCancelFriendsPage(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	training, err := getTraining(st, userCancelFriendTrainingID[sessionKey{userID, chatID}])
	if err != nil || training == nil {
		return nil, errors.New("training not found")
	}
	friends, err := getFriendsInTraining(st, training, userID)
	if err != nil || len(friends) == 0 {
		return nil, errors.New("friends not found")
	}
	page, _ := strconv.Atoi(data.Get("p"))
	text := fmt.Sprintf("[%s](tg://user?id=%d), какой друг не сможет пойти?", data.Get("username"), userID)
	keyboard := keyboards.PaginatedFriendsKeyboard("frnogo", "tr", 1, 2, page, friends)
	edit := tgbotapi.NewEditMessageText(chatID, messageID, text)
	edit.ParseMode = tgbotapi.ModeMarkdown
	edit.ReplyMarkup = &keyboard
	res := NewAction()
	res.AddEdit(&edit)
	user_state.SetUserState(userID, chatID, "frnogo_tr:1")
	return res, nil
}

func getFriendsInTraining(st storage.Storage, training *domain.Training, userID int64) ([]domain.Player, error) {
	ownedPlayers, err := st.GetPlayersByTelegramID(training.TeamID, userID)
	if err != nil || len(ownedPlayers) == 0 {
		return nil, err
	}
	ownerID := ownedPlayers[0].ID
	friends := make([]domain.Player, 0)
	for _, player := range training.Players {
		if player.TelegramID == userID && player.ID != ownerID {
			friends = append(friends, player)
		}
	}
	return friends, nil
}

func confirmFriendCancelTraining(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	trainingID := userCancelFriendTrainingID[sessionKey{userID, chatID}]
	playerID := data["id"]

	training, err := getTraining(st, trainingID)
	if err != nil {
		return nil, err
	}
	player, err := getOwnedPlayer(st, playerID, training.TeamID, userID)
	if err != nil {
		return nil, err
	}

	if err := st.RemoveTrainingPlayer(training.ID, player.ID); err != nil {
		return nil, err
	}
	training, err = getTraining(st, training.ID)
	if err != nil {
		return nil, err
	}

	res := NewAction()

	keyboard := keyboards.TakePartKeyboard("tr", training.ID)
	editMsg := tgbotapi.NewEditMessageText(chatID, userCancelContextMessage[sessionKey{userID, chatID}], training.String())
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

	delete(userCancelFriendTrainingID, sessionKey{userID, chatID})
	delete(userCancelContextMessage, sessionKey{userID, chatID})
	user_state.ClearUserState(userID, chatID)

	return res, nil
}
