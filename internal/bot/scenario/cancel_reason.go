package scenario

import (
	"fmt"
	"hockey_bot/internal/bot/user_state"
	"hockey_bot/internal/pendingreply"
	"hockey_bot/internal/storage"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// CancelReason сценарий для обработки текстового ответа пользователя
// на вопрос «почему отменил?».
type CancelReason struct {
	BaseScenario
}

var _ Scenario = (*CancelReason)(nil)

func NewCancelReason() *CancelReason {
	return &CancelReason{
		BaseScenario{
			id: "cancel_reason",
			steps: []Step{
				handleReasonReply,
			},
		},
	}
}

func handleReasonReply(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {

	trainingID := data.Get("training_id")
	teamID := data.Get("team_id")
	username := data.Get("username")
	if username == "" {
		username = fmt.Sprintf("Игрок %d", userID)
	}

	reason := data.Get("message")

	res := NewAction()

	// Благодарим пользователя за комментарий.
	replyToUser := tgbotapi.NewMessage(chatID, "🙏 Спасибо, что предупредил! Это очень помогает тренеру планировать состав 💪")
	replyToUser.ParseMode = tgbotapi.ModeMarkdown
	res.AddReply(&replyToUser)

	// Уведомляем тренеров (админов команды) с текстом причины.
	if teamID == "" {
		training, err := getTraining(st, trainingID)
		if err != nil {
			return nil, err
		}
		teamID = training.TeamID
	}
	team, err := st.GetTeamByID(teamID)
	if err != nil {
		return nil, err
	}
	if team == nil {
		return nil, ErrNotFound
	}
	player, err := st.GetPlayer(userID, team.ID)
	if err != nil {
		return nil, err
	}
	name := username
	if player != nil && player.FullName != "" {
		name = player.FullName
	}
	playerMention := fmt.Sprintf("[%s](tg://user?id=%d)", name, userID)
	textForCoach := fmt.Sprintf("%s отменил участие в тренировке со следующим сообщением: «%s»", playerMention, reason)
	for _, adminID := range team.Admins {
		msg := tgbotapi.NewMessage(adminID, textForCoach)
		msg.ParseMode = tgbotapi.ModeMarkdown
		res.AddReply(&msg)
	}

	// Пользователь ответил — больше не нужно срабатывать по таймауту.
	if trainingID != "" {
		if !pendingreply.ResolveRequest(userID, trainingID, data.Get("request_id")) {
			return nil, nil // таймер или другой ответ уже завершил это ожидание
		}
	}

	user_state.ClearUserState(userID, chatID)
	return res, nil
}
