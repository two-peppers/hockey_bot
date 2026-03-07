package scenario

import (
	"hockey_bot/internal/bot/keyboards"
	"hockey_bot/internal/bot/user_state"
	"hockey_bot/internal/domain"
	"hockey_bot/internal/storage"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type TrainingsList struct {
	BaseScenario
}

var _ Scenario = (*TrainingsList)(nil)

func NewTrainingsList() *TrainingsList {
	return &TrainingsList{
		BaseScenario{
			id: "trainings",
			steps: []Step{
				showUpcomingTrainings,
				selectUpcomingTraining,
			},
		},
	}
}

func showUpcomingTrainings(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	res := NewAction()

	list, err := st.GetUpcomingTrainingsByTelegramID(userID, 20)
	if err != nil {
		return nil, err
	}

	if len(list) == 0 {
		msg := tgbotapi.NewMessage(chatID, "Предстоящих тренировок нет.")
		msg.ParseMode = tgbotapi.ModeMarkdown
		res.AddReply(&msg)
		user_state.ClearUserState(userID, chatID)
		return res, nil
	}

	msg := tgbotapi.NewMessage(chatID, "Предстоящие тренировки")
	msg.ParseMode = tgbotapi.ModeMarkdown
	kb := keyboards.TrainingsChoiceKeyboard(list)
	msg.ReplyMarkup = kb
	res.AddReply(&msg)
	user_state.SetUserState(userID, chatID, "trainings:0")

	return res, nil
}

func selectUpcomingTraining(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	res := NewAction()
	trainingID := data.Get("id")
	if trainingID == "" {
		alert := tgbotapi.NewMessage(chatID, "Тренировка не найдена.")
		res.AddReply(&alert)
		user_state.ClearUserState(userID, chatID)
		return res, nil
	}

	training, err := st.GetTrainingByID(trainingID)
	if err != nil {
		return nil, err
	}
	if training == nil || training.Type == domain.TrainingMatch {
		alert := tgbotapi.NewMessage(chatID, "Тренировка не найдена.")
		res.AddReply(&alert)
		user_state.ClearUserState(userID, chatID)
		return res, nil
	}

	keyboard := keyboards.TakePartKeyboard("tr", training.ID)
	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, training.String())
	editMsg.ParseMode = tgbotapi.ModeMarkdown
	editMsg.ReplyMarkup = &keyboard
	res.AddEdit(&editMsg)
	user_state.ClearUserState(userID, chatID)

	return res, nil
}
