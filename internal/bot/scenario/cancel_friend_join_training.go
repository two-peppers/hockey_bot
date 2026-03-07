package scenario

import (
	"errors"
	"fmt"
	"hockey_bot/internal/bot/keyboards"
	"hockey_bot/internal/bot/user_state"
	"hockey_bot/internal/storage"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type CancelFriendJoinTraining struct {
	BaseScenario
}

var _ Scenario = (*CancelFriendJoinTraining)(nil)

var userCancelFriendTrainingID = make(map[int64]string)
var userCancelContextMessage = make(map[int64]int)

func NewCancelFriendJoinTraining() *CancelFriendJoinTraining {
	return &CancelFriendJoinTraining{
		BaseScenario{
			id: "frnogo",
			steps: []Step{
				askFriendNameForCancel,
				confirmFriendCancelTraining,
			},
		},
	}
}

func askFriendNameForCancel(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	userCancelContextMessage[userID] = messageID

	trainingID := data["id"]
	training, _ := st.GetTrainingByID(trainingID)

	userCancelFriendTrainingID[userID] = trainingID

	// Получаем ВСЕХ друзей этого пользователя в тренировке
	friendsInTraining, err := st.GetPlayersByTelegramID(training.TeamID, userID)

	if len(friendsInTraining) == 0 {
		return nil, errors.New("no players for user")
	}

	friendsInTraining = friendsInTraining[1:]
	fmt.Printf("Training friends %d\n", len(friendsInTraining))

	if err != nil || len(friendsInTraining) == 0 {
		return nil, errors.New("no friends in training")
	}

	replyMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("[%s](tg://user?id=%d), какой друг не сможет пойти?", data.Get("username"), userID))
	replyMsg.ParseMode = tgbotapi.ModeMarkdown
	replyMsg.ReplyMarkup = keyboards.FriendsKeyboard("frnogo", "tr", 1, friendsInTraining)

	user_state.SetUserState(userID, chatID, "frnogo:1")

	res := NewAction()
	res.AddTemporaryReply(&replyMsg)

	return res, nil
}

func confirmFriendCancelTraining(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	trainingID := userCancelFriendTrainingID[userID]
	playerID := data["id"]

	training, _ := st.GetTrainingByID(trainingID)
	player, _ := st.GetPlayerByID(playerID)

	training.RemovePlayer(player)
	st.SaveTraining(training)

	res := NewAction()

	keyboard := keyboards.TakePartKeyboard("tr", training.ID)
	editMsg := tgbotapi.NewEditMessageText(chatID, userCancelContextMessage[userID], training.String())
	editMsg.ParseMode = tgbotapi.ModeMarkdown
	editMsg.ReplyMarkup = &keyboard

	res.AddEdit(&editMsg)
	if chatID > 0 && training.Team.ChatID != nil && *training.Team.ChatID != 0 && training.TeamMessageID != 0 {
		teamEdit := tgbotapi.NewEditMessageText(*training.Team.ChatID, training.TeamMessageID, training.String())
		teamEdit.ParseMode = tgbotapi.ModeMarkdown
		teamEdit.ReplyMarkup = &keyboard
		res.AddEdit(&teamEdit)
	}
	res.FlushTemporary = true

	user_state.ClearUserState(userID, chatID)

	return res, nil
}
