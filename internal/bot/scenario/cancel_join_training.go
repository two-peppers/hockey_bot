package scenario

import (
	"errors"
	"hockey_bot/internal/bot/keyboards"
	"hockey_bot/internal/storage"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type CancelJoinTraining struct {
	BaseScenario
}

var _ Scenario = (*CancelJoinTraining)(nil)

func NewCancelJoinTraining() *CancelJoinTraining {
	return &CancelJoinTraining{
		BaseScenario{
			id: "nogo_tr",
			steps: []Step{
				confirmTakePartTrainingCancel,
			},
		},
	}
}

func confirmTakePartTrainingCancel(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	trainingID := data.Get("id")
	training, err := st.GetTrainingByID(trainingID)
	if err != nil {
		return nil, err
	}

	player, err := st.GetPlayer(userID, training.TeamID)

	if err != nil || player == nil {
		return nil, errors.New("no player for user")
	}

	training.RemovePlayer(player)
	st.SaveTraining(training)

	keyboard := keyboards.TakePartKeyboard("tr", training.ID)
	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, training.String())
	editMsg.ParseMode = tgbotapi.ModeMarkdown
	editMsg.ReplyMarkup = &keyboard

	res := NewAction()
	res.AddEdit(&editMsg)

	return res, nil
}
