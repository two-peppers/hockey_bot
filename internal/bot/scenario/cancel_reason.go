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
	defer user_state.ClearUserState(userID, chatID)

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
	if teamID == "" && trainingID != "" {
		// Попробуем восстановить teamID из тренировки, если его нет в data.
		if training, err := st.GetTrainingByID(trainingID); err == nil && training != nil {
			teamID = training.TeamID
		}
	}

	if teamID != "" {
		team, err := st.GetTeamByID(teamID)
		if err == nil && team != nil {
			// Пытаемся получить фамилию/имя игрока для красивой ссылки.
			name := ""
			if player, err := st.GetPlayer(userID, team.ID); err == nil && player != nil && player.FullName != "" {
				name = player.FullName
			}
			if name == "" {
				name = username
			}
			if name == "" {
				name = fmt.Sprintf("Игрок %d", userID)
			}

			playerMention := fmt.Sprintf("[%s](tg://user?id=%d)", name, userID)
			textForCoach := fmt.Sprintf("%s отменил участие в тренировке со следующим сообщением: «%s»", playerMention, reason)

			for _, adminID := range team.Admins {
				msg := tgbotapi.NewMessage(adminID, textForCoach)
				msg.ParseMode = tgbotapi.ModeMarkdown
				res.AddReply(&msg)
			}
		}
	}

	// Пользователь ответил — больше не нужно срабатывать по таймауту.
	if trainingID != "" {
		pendingreply.Resolve(userID, trainingID)
	}

	return res, nil
}

