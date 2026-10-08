package bot

import (
	"encoding/json"
	"errors"
	"fmt"
	"hockey_bot/internal/bot/keyboards"
	"hockey_bot/internal/bot/scenario"
	state "hockey_bot/internal/bot/user_state"
	"hockey_bot/internal/pendingreply"
	"hockey_bot/internal/storage"
	"log"
	"strconv"
	"strings"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type RouteMap map[string]scenario.Scenario

func getScenario(scenarioID string) (scenario.Scenario, error) {
	routeMap := RouteMap{
		"create_team":    scenario.NewCreateTeam(),
		"manage_team":    scenario.NewManageTeam(),
		"train":          scenario.NewScheduleTraining(),
		"game":           scenario.NewScheduleGame(),
		"go_tr":          scenario.NewJoinTraining(),
		"nogo_tr":        scenario.NewCancelJoinTraining(),
		"frgo_tr":        scenario.NewFriendJoinTraining(),
		"frnogo_tr":      scenario.NewCancelFriendJoinTraining(),
		"add_chat":       scenario.NewAddBot(),
		"players":        scenario.NewPlayersList(),
		"set_admin":      scenario.NewSetAdmin(),
		"del_admin":      scenario.NewDelAdmin(),
		"cancel_reason":  scenario.NewCancelReason(),
		"approve_lineup": scenario.NewApproveLineup(),
		"trainings":      scenario.NewTrainingsList(),
	}

	if sc, exists := routeMap[scenarioID]; exists {
		return sc, nil
	}

	return nil, fmt.Errorf("scenario id %s not found", scenarioID)
}

func executeRoute(bot *tgbotapi.BotAPI, store storage.Storage, chatID int64, userID int64, messageID int, userName string, message string) *scenario.Action {
	sc, st, dt := route(userID, chatID)

	if len(sc) == 0 {
		return nil
	}

	if len(message) > 0 {
		dt.Set("message", message)
	}

	dt.Set("username", userName)

	scen, err := getScenario(sc)

	if err != nil {
		state.ClearUserState(userID, chatID)
		log.Printf("error getting scenario: %v", err)
		return nil
	}

	step, err := scen.GetStep(st)

	if err != nil {
		state.ClearUserState(userID, chatID)
		log.Printf("error getting step: %v", err)
		return nil
	}

	action, err := step(store, chatID, userID, messageID, dt)

	if err != nil {
		log.Printf("scenario %s step %d user %d chat %d: %v", sc, st, userID, chatID, err)
		if action == nil || len(action.Reply) == 0 {
			action = scenario.NewAction()
			text := "Не удалось выполнить действие. Попробуй ещё раз."
			if errors.Is(err, scenario.ErrForbidden) || errors.Is(err, scenario.ErrNotFound) {
				text = err.Error()
				state.ClearUserState(userID, chatID)
			}
			msg := tgbotapi.NewMessage(chatID, text)
			action.AddReply(&msg)
		}
	}

	return executeAction(bot, store, chatID, userID, action)
}

// Обновления составов из разных чатов отправляем последовательно и всегда
// перечитываем БД: более старый Action не затрёт новое состояние сообщения.
var trainingRenderMu sync.Mutex

func executeAction(bot *tgbotapi.BotAPI, store storage.Storage, chatID, userID int64, action *scenario.Action) *scenario.Action {
	if action == nil {
		return nil
	}

	// Если задан алерт для callback — остальное не выполняем, алерт покажет handleCallback
	if action.CallbackAlert != "" {
		return action
	}

	if action.RefreshTrainingID != "" {
		trainingRenderMu.Lock()
		defer trainingRenderMu.Unlock()
		training, err := store.GetTrainingByID(action.RefreshTrainingID)
		if err != nil || training == nil {
			log.Printf("failed to reload training %s for display: %v", action.RefreshTrainingID, err)
			msg := tgbotapi.NewMessage(chatID, "Запись обновлена, но состав не удалось загрузить. Открой тренировку заново.")
			action.AddReply(&msg)
			action.Edit = nil
		} else {
			keyboard := keyboards.TakePartKeyboard("tr", training.ID)
			for _, edit := range action.Edit {
				edit.Text = training.String()
				edit.ReplyMarkup = &keyboard
			}
		}
	}

	if len(action.Reply) > 0 {
		for _, reply := range action.Reply {
			reply.DisableNotification = true
			sentMsg, err := bot.Send(reply)
			if err != nil {
				log.Printf("error sending reply: %v", err)
				continue
			}
			// Если нужно сохранить message_id утверждённого состава для тренировки
			if action.ApprovedMessageTrainingID != "" {
				if err := store.UpdateApprovedMessageID(action.ApprovedMessageTrainingID, sentMsg.MessageID); err != nil {
					log.Printf("error updating approved message id for training %s: %v", action.ApprovedMessageTrainingID, err)
				}
				// Сохраняем только для одного сообщения, чтобы не перезаписывать случайно
				action.ApprovedMessageTrainingID = ""
			}
		}
	}

	if len(action.TemporaryReply) > 0 {
		for _, reply := range action.TemporaryReply {
			reply.DisableNotification = true
			sentMsg, err := bot.Send(reply)
			if err == nil {
				state.AddToMessageBuffer(sentMsg.Chat.ID, userID, sentMsg.MessageID)

				// Если для действия задан контекст ожидаемого ответа,
				// регистрируем его с учетом фактических данных отправленного сообщения.
				if action.PendingReplyContext != nil && reply.ChatID == action.PendingReplyContext.QuestionChatID {
					ctx := *action.PendingReplyContext
					ctx.QuestionChatID = sentMsg.Chat.ID
					ctx.QuestionMessageID = sentMsg.MessageID
					pendingreply.Register(bot, store, &ctx)
				}
			} else {
				log.Printf("failed to send temporary reply to chat %d: %v", reply.ChatID, err)
				if action.CoachNotificationOnDMFailure != nil && reply.ChatID == action.CoachNotificationOnDMFailure.QuestionChatID {
					// Личное сообщение игроку не доставлено — уведомляем тренеров через Action.
					log.Printf("failed to send cancel reason question to user %d: %v", reply.ChatID, err)
					// Ответ уже не ожидается: не оставляем сценарий причины отмены,
					// который мог бы обработать следующее случайное сообщение пользователя.
					state.ClearUserStateIf(
						action.CoachNotificationOnDMFailure.UserID,
						action.CoachNotificationOnDMFailure.QuestionChatID,
						action.CoachNotificationOnDMFailure.ReplyState,
					)
					notifyCoachesCancelWithoutDM(bot, store, action.CoachNotificationOnDMFailure)
				}
			}
		}
	}

	if len(action.Edit) > 0 {
		for _, edit := range action.Edit {
			if _, err := bot.Request(edit); err != nil && !strings.Contains(err.Error(), "message is not modified") {
				log.Printf("failed to edit message %d in chat %d: %v", edit.MessageID, edit.ChatID, err)
				msg := tgbotapi.NewMessage(chatID, "Не удалось обновить сообщение. Открой тренировку или меню заново, чтобы увидеть актуальные данные.")
				if _, sendErr := bot.Send(msg); sendErr != nil {
					log.Printf("failed to report edit error: %v", sendErr)
				}
			}
		}
	}

	if len(action.Delete) > 0 {
		for _, del := range action.Delete {
			requestLogged(bot, del, "delete message")
		}
	}

	if len(action.Pin) > 0 {
		for _, pin := range action.Pin {
			sentMsg, err := bot.Send(pin)
			if err != nil {
				log.Printf("failed to publish pinned message to chat %d: %v", pin.ChatID, err)
				msg := tgbotapi.NewMessage(chatID, "Данные сохранены, но опубликовать сообщение в чате не удалось. Проверь права бота.")
				if _, sendErr := bot.Send(msg); sendErr != nil {
					log.Printf("failed to report publication error: %v", sendErr)
				}
			}

			if err == nil {
				pinReq := tgbotapi.PinChatMessageConfig{
					ChatID:              sentMsg.Chat.ID,
					MessageID:           sentMsg.MessageID,
					DisableNotification: true,
				}

				requestLogged(bot, pinReq, "pin message")

				// Если нужно сохранить message_id исходного сообщения тренировки в командном чате
				if action.TeamMessageTrainingID != "" {
					if err := store.UpdateTeamMessageID(action.TeamMessageTrainingID, sentMsg.MessageID); err != nil {
						log.Printf("error updating team message id for training %s (pin): %v", action.TeamMessageTrainingID, err)
					}
					action.TeamMessageTrainingID = ""
				}

				// Если нужно сохранить message_id утверждённого состава для тренировки
				if action.ApprovedMessageTrainingID != "" {
					if err := store.UpdateApprovedMessageID(action.ApprovedMessageTrainingID, sentMsg.MessageID); err != nil {
						log.Printf("error updating approved message id for training %s (pin): %v", action.ApprovedMessageTrainingID, err)
					}
					action.ApprovedMessageTrainingID = ""
				}
			}
		}
	}

	if action.FlushTemporary {
		for _, msg := range state.DrainMessageBuffer(userID, chatID) {
			delMsg := tgbotapi.NewDeleteMessage(msg.ChatID, msg.MessageID)
			requestLogged(bot, delMsg, "delete temporary message")
		}
	}

	if action.LeaveChat {
		leave := tgbotapi.LeaveChatConfig{
			ChatID: chatID,
		}

		requestLogged(bot, leave, "leave chat")
	}

	return action
}

func requestLogged(bot *tgbotapi.BotAPI, request tgbotapi.Chattable, operation string) {
	if _, err := bot.Request(request); err != nil {
		log.Printf("%s: %v", operation, err)
	}
}

func parseRoute(value string) (string, int, scenario.Data, error) {
	parts := strings.SplitN(value, ":", 3)
	if len(parts) < 2 {
		return "", 0, nil, fmt.Errorf("invalid route")
	}
	step, err := strconv.Atoi(parts[1])
	if err != nil || step < 0 {
		return "", 0, nil, fmt.Errorf("invalid step")
	}
	scen, err := getScenario(parts[0])
	if err != nil {
		return "", 0, nil, err
	}
	if _, err := scen.GetStep(step); err != nil {
		return "", 0, nil, err
	}
	data := make(scenario.Data)
	if len(parts) == 3 {
		if err := json.Unmarshal([]byte(parts[2]), &data); err != nil {
			return "", 0, nil, err
		}
	}
	return parts[0], step, data, nil
}

func route(userID int64, chatID int64) (string, int, scenario.Data) {
	value := state.GetUserState(userID, chatID)
	if value == "" {
		return "", 0, nil
	}
	scen, step, data, err := parseRoute(value)
	if err != nil {
		log.Printf("invalid route for user %d in chat %d: %v", userID, chatID, err)
		state.ClearUserState(userID, chatID)
		return "", 0, nil
	}
	return scen, step, data
}

// notifyCoachesCancelWithoutDM отправляет тренерам уведомление, что игрок отменил тренировку,
// но личное сообщение ему не удалось доставить (причина неизвестна).
func notifyCoachesCancelWithoutDM(bot *tgbotapi.BotAPI, store storage.Storage, ctx *pendingreply.PendingReplyContext) {
	if ctx == nil || ctx.TeamID == "" {
		return
	}
	team, err := store.GetTeamByID(ctx.TeamID)
	if err != nil || team == nil {
		if err != nil {
			log.Printf("notifyCoachesCancelWithoutDM: failed to load team %s: %v", ctx.TeamID, err)
		}
		return
	}
	name := ctx.PlayerFullName
	if name == "" {
		name = ctx.Username
	}
	if name == "" {
		name = fmt.Sprintf("Игрок %d", ctx.UserID)
	}
	playerMention := fmt.Sprintf("[%s](tg://user?id=%d)", name, ctx.UserID)
	text := fmt.Sprintf(
		"❌ %s отменил тренировку, но я не смог написать ему в личку, поэтому причина неизвестна.",
		playerMention,
	)
	for _, adminID := range team.Admins {
		msg := tgbotapi.NewMessage(adminID, text)
		msg.ParseMode = tgbotapi.ModeMarkdown
		if _, err := bot.Send(msg); err != nil {
			log.Printf("failed to notify admin %d about cancel without DM: %v", adminID, err)
		}
	}
}
