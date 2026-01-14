package bot

import (
	"encoding/json"
	"fmt"
	"hockey_bot/internal/bot/scenario"
	state "hockey_bot/internal/bot/user_state"
	"hockey_bot/internal/storage"
	"log"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type RouteMap map[string]scenario.Scenario

func getScenario(scenarioID string) (scenario.Scenario, error) {
	routeMap := RouteMap{
		"create_team": scenario.NewCreateTeam(),
		"manage_team": scenario.NewManageTeam(),
		"train":       scenario.NewScheduleTraining(),
		"go_tr":       scenario.NewJoinTraining(),
		"nogo_tr":     scenario.NewCancelJoinTraining(),
		"frgo_tr":     scenario.NewFriendJoinTraining(),
		"frnogo_tr":   scenario.NewCancelFriendJoinTraining(),
		"add_chat":    scenario.NewAddBot(),
		"players":     scenario.NewPlayersList(),
		"set_admin":   scenario.NewSetAdmin(),
		"del_admin":   scenario.NewDelAdmin(),
	}

	if sc, exists := routeMap[scenarioID]; exists {
		return sc, nil
	}

	return nil, fmt.Errorf("scenario id %s not found", scenarioID)
}

func executeRoute(bot *tgbotapi.BotAPI, store storage.Storage, chatID int64, userID int64, messageID int, userName string, message string) {
	sc, st, dt := route(userID, chatID)

	if len(sc) == 0 {
		return
	}

	if len(message) > 0 {
		dt.Set("message", message)
	}

	dt.Set("username", userName)

	scen, err := getScenario(sc)

	if err != nil {
		state.ClearUserState(userID, chatID)
		log.Printf("error getting scenario: %v", err)
		return
	}

	step, err := scen.GetStep(st)

	if err != nil {
		state.ClearUserState(userID, chatID)
		log.Printf("error getting step: %v", err)
	}

	action, err := step(store, chatID, userID, messageID, dt)

	if err != nil {
		return
	}

	if len(action.Reply) > 0 {
		for _, reply := range action.Reply {
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
