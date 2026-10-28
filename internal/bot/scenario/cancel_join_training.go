package scenario

import (
	"encoding/json"
	"errors"
	"fmt"
	"hockey_bot/internal/bot/keyboards"
	"hockey_bot/internal/bot/user_state"
	"hockey_bot/internal/pendingreply"
	"hockey_bot/internal/storage"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
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
	// Кнопка запускает одношаговый сценарий. Если не очистить его состояние,
	// следующее обычное сообщение пользователя в этом чате повторно выполнит
	// отмену и снова отправит запрос причины в личку.
	user_state.ClearUserState(userID, chatID)

	trainingID := data.Get("id")
	training, err := getTraining(st, trainingID)
	if err != nil {
		return nil, err
	}

	player, err := st.GetPlayer(userID, training.TeamID)

	if err != nil || player == nil {
		return nil, errors.New("no player for user")
	}

	if err := st.RemoveTrainingPlayer(training.ID, player.ID); err != nil {
		return nil, err
	}
	training, err = getTraining(st, training.ID)
	if err != nil {
		return nil, err
	}

	username := data.Get("username")
	if username == "" {
		username = fmt.Sprintf("Игрок %d", userID)
	}

	// Готовим данные для сценария причины отмены в личном чате.
	requestID := uuid.NewString()
	ctxData := make(Data)
	ctxData.Set("request_id", requestID)
	ctxData.Set("training_id", training.ID)
	ctxData.Set("team_id", training.TeamID)
	ctxData.Set("username", username)

	raw, err := json.Marshal(ctxData)
	if err != nil {
		return nil, err
	}
	replyState := fmt.Sprintf("cancel_reason:0:%s", raw)
	user_state.SetUserState(userID, userID, replyState)

	keyboard := keyboards.TakePartKeyboard("tr", training.ID)
	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, training.String())
	editMsg.ParseMode = tgbotapi.ModeMarkdown
	editMsg.ReplyMarkup = &keyboard

	res := NewAction()
	res.RefreshTrainingID = training.ID
	res.AddEdit(&editMsg)
	if chatID > 0 && training.Team.ChatID != nil && *training.Team.ChatID != 0 && training.TeamMessageID != 0 {
		teamEdit := tgbotapi.NewEditMessageText(*training.Team.ChatID, training.TeamMessageID, training.String())
		teamEdit.ParseMode = tgbotapi.ModeMarkdown
		teamEdit.ReplyMarkup = &keyboard
		res.AddEdit(&teamEdit)
	}

	reasonMsg := tgbotapi.NewMessage(
		userID,
		"👋 Привет! Ты отменил участие в тренировке.\n\n"+
			"🤔 Можешь написать, почему?",
	)
	reasonMsg.ParseMode = tgbotapi.ModeMarkdown
	res.AddTemporaryReply(&reasonMsg)

	// Настраиваем контекст ожидания ответа для последующего трекинга.
	ctx := &pendingreply.PendingReplyContext{
		RequestID:      requestID,
		ReplyState:     replyState,
		UserID:         userID,
		Username:       username,
		PlayerFullName: player.FullName,
		TrainingID:     training.ID,
		TeamID:         training.TeamID,
		QuestionChatID: userID,
	}
	res.PendingReplyContext = ctx

	// Если личное сообщение не доставится, уведомим тренеров
	res.CoachNotificationOnDMFailure = ctx

	return res, nil
}
