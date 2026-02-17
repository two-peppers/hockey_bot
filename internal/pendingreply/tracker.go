package pendingreply

import (
	"fmt"
	"hockey_bot/internal/storage"
	"log"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// PendingReplyContext описывает ожидание ответа пользователя на отправленное сообщение.
// Сейчас используется для причины отмены тренировки, но может быть переиспользован
// в других сценариях, где нужно дождаться ответа и по таймауту выполнить действие.
type PendingReplyContext struct {
	UserID            int64
	Username          string
	PlayerFullName    string
	TrainingID        string
	TeamID            string
	QuestionChatID    int64
	QuestionMessageID int
}

type key struct {
	UserID     int64
	TrainingID string
}

var (
	mu       sync.Mutex
	pending  = make(map[key]*PendingReplyContext)
	timeout  = 10 * time.Minute
)

// Register регистрирует ожидание ответа и запускает таймер.
func Register(bot *tgbotapi.BotAPI, store storage.Storage, ctx *PendingReplyContext) {
	if ctx == nil {
		return
	}

	k := key{
		UserID:     ctx.UserID,
		TrainingID: ctx.TrainingID,
	}

	mu.Lock()
	pending[k] = ctx
	mu.Unlock()

	go func(localCtx *PendingReplyContext, localKey key) {
		time.Sleep(timeout)

		mu.Lock()
		current, ok := pending[localKey]
		if !ok || current != localCtx {
			// Уже обработано или переопределено новым ожиданием.
			mu.Unlock()
			return
		}
		// Удаляем из pending перед выполнением действий.
		delete(pending, localKey)
		mu.Unlock()

		// Удаляем вопрос пользователю, если можем.
		if localCtx.QuestionChatID != 0 && localCtx.QuestionMessageID != 0 {
			del := tgbotapi.NewDeleteMessage(localCtx.QuestionChatID, localCtx.QuestionMessageID)
			if _, err := bot.Request(del); err != nil {
				log.Printf("failed to delete cancel reason question message: %v", err)
			}
		}

		// Отправляем тренерам уведомление без комментария.
		team, err := store.GetTeamByID(localCtx.TeamID)
		if err != nil || team == nil {
			if err != nil {
				log.Printf("failed to load team for cancel reason timeout: %v", err)
			}
			return
		}

		// Формируем ссылку на игрока: [Фамилия](tg://user?id=USER_ID).
		name := localCtx.PlayerFullName
		if name == "" {
			name = localCtx.Username
		}
		if name == "" {
			name = fmt.Sprintf("Игрок %d", localCtx.UserID)
		}
		playerMention := fmt.Sprintf("[%s](tg://user?id=%d)", name, localCtx.UserID)

		text := fmt.Sprintf(
			"❌ %s отменил участие в тренировке.\n\n😐 Игрок не оставил комментарий.",
			playerMention,
		)

		for _, adminID := range team.Admins {
			msg := tgbotapi.NewMessage(adminID, text)
			msg.ParseMode = tgbotapi.ModeMarkdown
			if _, err := bot.Send(msg); err != nil {
				log.Printf("failed to notify admin %d about cancel without reason: %v", adminID, err)
			}
		}
	}(ctx, k)
}

// Resolve помечает ожидание как выполненное (пользователь ответил), чтобы
// по таймауту не отправлять уведомление «без комментариев».
func Resolve(userID int64, trainingID string) {
	mu.Lock()
	defer mu.Unlock()

	k := key{
		UserID:     userID,
		TrainingID: trainingID,
	}

	delete(pending, k)
}
