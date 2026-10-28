package pendingreply

import (
	"fmt"
	"hockey_bot/internal/bot/user_state"
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
	RequestID         string
	ReplyState        string
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
	mu      sync.Mutex
	pending = make(map[key]*pendingEntry)
	timeout = 10 * time.Minute
)

type pendingEntry struct {
	context PendingReplyContext
	timer   *time.Timer
}

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
	if old := pending[k]; old != nil {
		old.timer.Stop()
	}
	entry := &pendingEntry{context: *ctx}
	pending[k] = entry
	entry.timer = time.AfterFunc(timeout, func() {
		localCtx := &entry.context

		mu.Lock()
		current, ok := pending[k]
		if !ok || current != entry {
			// Уже обработано или переопределено новым ожиданием.
			mu.Unlock()
			return
		}
		// Удаляем из pending перед выполнением действий.
		delete(pending, k)
		mu.Unlock()

		if localCtx.ReplyState != "" {
			user_state.ClearUserStateIf(localCtx.UserID, localCtx.QuestionChatID, localCtx.ReplyState)
		}

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
	})
	mu.Unlock()
}

// Resolve помечает ожидание как выполненное (пользователь ответил), чтобы
// по таймауту не отправлять уведомление «без комментариев».
func Resolve(userID int64, trainingID string) bool {
	return resolve(userID, trainingID, "")
}

// ResolveRequest не позволяет ответу на старый вопрос отменить новое ожидание.
func ResolveRequest(userID int64, trainingID, requestID string) bool {
	return resolve(userID, trainingID, requestID)
}

func resolve(userID int64, trainingID, requestID string) bool {
	mu.Lock()
	defer mu.Unlock()
	k := key{UserID: userID, TrainingID: trainingID}
	entry, ok := pending[k]
	if !ok || (requestID != "" && entry.context.RequestID != requestID) {
		return false
	}
	delete(pending, k)
	entry.timer.Stop()
	return true
}
