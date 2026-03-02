package bot

import (
	"encoding/json"
	"fmt"
	"hockey_bot/internal/bot/scenario"
	state "hockey_bot/internal/bot/user_state"
	"hockey_bot/internal/pendingreply"
	"hockey_bot/internal/storage"
	"log"
	"strconv"
	"strings"

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
		return nil
	}

	if action == nil {
		return nil
	}

	// Если задан алерт для callback — остальное не выполняем, алерт покажет handleCallback
	if action.CallbackAlert != "" {
		return action
	}

	if len(action.Reply) > 0 {
		for _, reply := range action.Reply {
			reply.DisableNotification = true
			_, err := bot.Send(reply)
			if err != nil {
				log.Printf("error sending reply: %v", err)
			}
		}
	}

	if len(action.TemporaryReply) > 0 {
		for _, reply := range action.TemporaryReply {
			reply.DisableNotification = true
			sentMsg, err := bot.Send(reply)
			if err == nil {
				state.AddToMessageBuffer(chatID, userID, sentMsg.MessageID)

				// Если для действия задан контекст ожидаемого ответа,
				// регистрируем его с учетом фактических данных отправленного сообщения.
				if action.PendingReplyContext != nil && reply.ChatID == action.PendingReplyContext.QuestionChatID {
					ctx := *action.PendingReplyContext
					ctx.QuestionChatID = sentMsg.Chat.ID
					ctx.QuestionMessageID = sentMsg.MessageID
					pendingreply.Register(bot, store, &ctx)
				}
			} else if action.CoachNotificationOnDMFailure != nil && reply.ChatID == action.CoachNotificationOnDMFailure.QuestionChatID {
				// Личное сообщение игроку не доставлено — уведомляем тренеров через Action.
				log.Printf("failed to send cancel reason question to user %d: %v", reply.ChatID, err)
				notifyCoachesCancelWithoutDM(bot, store, action.CoachNotificationOnDMFailure)
			}
		}
	}

	if len(action.Edit) > 0 {
		for _, edit := range action.Edit {
			go bot.Send(edit)
		}
	}

	if len(action.Delete) > 0 {
		for _, del := range action.Delete {
			go bot.Request(del)
		}
	}

	if len(action.Pin) > 0 {
		for _, pin := range action.Pin {
			sentMsg, err := bot.Send(pin)

			if err == nil {
				pinReq := tgbotapi.PinChatMessageConfig{
					ChatID:              sentMsg.Chat.ID,
					MessageID:           sentMsg.MessageID,
					DisableNotification: true,
				}

				bot.Request(pinReq)
			}
		}
	}

	if action.FlushTemporary {
		for _, msg := range state.GetUserMessageBuffer(userID) {
			delMsg := tgbotapi.NewDeleteMessage(msg.ChatID, msg.MessageID)
			go bot.Send(delMsg)
		}
	}

	if action.LeaveChat {
		leave := tgbotapi.LeaveChatConfig{
			ChatID: chatID,
		}

		go bot.Send(leave)
	}

	return action
}

func route(userID int64, chatID int64) (string, int, scenario.Data) {
	userState := state.GetUserState(userID, chatID)

	if len(userState) > 0 {
		log.Printf("Current user state: %s", userState)
	} else {
		log.Printf("Current user state is empty")
	}

	if userState == "" {
		return "", 0, nil
	}

	parts := strings.SplitN(userState, ":", 3)

	if len(parts) < 2 {
		state.ClearUserState(userID, chatID)
		return "", 0, nil
	}

	scen := parts[0]

	step, err := strconv.Atoi(parts[1])
	if err != nil {
		return scen, 0, nil
	}

	data := make(scenario.Data)

	if len(parts) == 3 && parts[2] != "" {
		if err := json.Unmarshal([]byte(parts[2]), &data); err != nil {
			return scen, step, nil
		}
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
