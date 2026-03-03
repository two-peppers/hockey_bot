package scenario

import (
	"fmt"
	"hockey_bot/internal/bot/keyboards"
	"hockey_bot/internal/bot/user_state"
	"hockey_bot/internal/domain"
	"hockey_bot/internal/storage"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type ApproveLineup struct {
	BaseScenario
}

var _ Scenario = (*ApproveLineup)(nil)

// Состояние утверждения состава для одного тренера (чтобы умещаться в 64 байта callback)
type approveLineupState struct {
	TeamID      string
	GameID      string
	ApprovedIDs map[string]bool
}

var approveLineupStateByUser = make(map[int64]*approveLineupState)
var approveLineupMessageID = make(map[int64]int)

func NewApproveLineup() *ApproveLineup {
	return &ApproveLineup{
		BaseScenario: BaseScenario{
			id: "approve_lineup",
			steps: []Step{
				approveLineupStep0FindGame,
				approveLineupStep1Manage,
				approveLineupStep2Confirm,
			},
		},
	}
}

func approveLineupStep0FindGame(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	res := NewAction()
	teamID := data.Get("team_id")
	if teamID == "" {
		teamID = data.Get("ti")
	}
	if teamID == "" {
		res.AddReply(&tgbotapi.MessageConfig{
			BaseChat: tgbotapi.BaseChat{ChatID: chatID},
			Text:     "Не указана команда.",
		})
		user_state.ClearUserState(userID, chatID)
		return res, nil
	}

	games, err := st.GetUpcomingGames(teamID, 10)
	if err != nil {
		return nil, err
	}
	if len(games) == 0 {
		msg := tgbotapi.NewMessage(chatID, "Нет предстоящих игр. Сначала запланируйте игру.")
		msg.ParseMode = tgbotapi.ModeMarkdown
		res.AddReply(&msg)
		user_state.ClearUserState(userID, chatID)
		return res, nil
	}

	if len(games) == 1 {
		g := &games[0]
		approvedIDs, _ := st.GetApprovedPlayerIDs(g.ID)
		approvedMap := make(map[string]bool)
		for _, id := range approvedIDs {
			approvedMap[id] = true
		}
		approveLineupStateByUser[userID] = &approveLineupState{
			TeamID:      teamID,
			GameID:      g.ID,
			ApprovedIDs: approvedMap,
		}
		user_state.SetUserState(userID, chatID, fmt.Sprintf("approve_lineup:1:{\"ti\":\"%s\",\"gi\":\"%s\"}", teamID, g.ID))
		return approveLineupStep1Manage(st, chatID, userID, messageID, data)
	}

	// Несколько игр — показать выбор
	msg := tgbotapi.NewMessage(chatID, "Выбери игру для утверждения состава:")
	msg.ParseMode = tgbotapi.ModeMarkdown
	msg.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{
		InlineKeyboard: keyboards.ApproveLineupGameChoiceKeyboard(teamID, games).InlineKeyboard,
	}
	res.AddTemporaryReply(&msg)
	user_state.SetUserState(userID, chatID, fmt.Sprintf("approve_lineup:0:{\"ti\":\"%s\"}", teamID))
	return res, nil
}

func approveLineupStep1Manage(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	res := NewAction()
	teamID := data.Get("team_id")
	if teamID == "" {
		teamID = data.Get("ti")
	}
	gameID := data.Get("game_id")
	if gameID == "" {
		gameID = data.Get("gi")
	}
	if gameID == "" {
		gameID = data.Get("g")
	}
	if gameID == "" {
		if s, ok := approveLineupStateByUser[userID]; ok {
			gameID = s.GameID
		}
	}
	if gameID == "" {
		res.AddReply(&tgbotapi.MessageConfig{
			BaseChat: tgbotapi.BaseChat{ChatID: chatID},
			Text:     "Игра не выбрана.",
		})
		user_state.ClearUserState(userID, chatID)
		return res, nil
	}

	game, err := st.GetTrainingByID(gameID)
	if err != nil || game == nil {
		res.AddReply(&tgbotapi.MessageConfig{
			BaseChat: tgbotapi.BaseChat{ChatID: chatID},
			Text:     "Игру не удалось загрузить.",
		})
		user_state.ClearUserState(userID, chatID)
		return res, nil
	}
	if teamID == "" {
		teamID = game.TeamID
	}
	if game.Type != domain.TrainingMatch {
		res.AddReply(&tgbotapi.MessageConfig{
			BaseChat: tgbotapi.BaseChat{ChatID: chatID},
			Text:     "Это не игра.",
		})
		user_state.ClearUserState(userID, chatID)
		return res, nil
	}
	if len(game.Players) == 0 {
		msg := tgbotapi.NewMessage(chatID, "Пока не набралось желающих.")
		res.AddReply(&msg)
		user_state.ClearUserState(userID, chatID)
		return res, nil
	}

	// Восстановить/инициализировать состояние утверждённых (в т.ч. из БД при повторном заходе)
	state, ok := approveLineupStateByUser[userID]
	if !ok || state.GameID != gameID {
		state = &approveLineupState{
			TeamID:      teamID,
			GameID:      gameID,
			ApprovedIDs: make(map[string]bool),
		}
		// Подгрузить уже утверждённых из БД
		approvedIDs, _ := st.GetApprovedPlayerIDs(gameID)
		for _, id := range approvedIDs {
			state.ApprovedIDs[id] = true
		}
		approveLineupStateByUser[userID] = state
	}

	// Toggle игрока по callback
	toggleID := data.Get("t")
	if toggleID != "" {
		if state.ApprovedIDs[toggleID] {
			delete(state.ApprovedIDs, toggleID)
		} else {
			state.ApprovedIDs[toggleID] = true
		}
		// Первый callback от нашего сообщения (нажатие на игрока) — сохраняем ID сообщения с составом
		if approveLineupMessageID[userID] == 0 && messageID > 0 {
			approveLineupMessageID[userID] = messageID
		}
	}

	approvedSet := state.ApprovedIDs
	text := approvedLineupMessage(game, state.approvedList(game))
	kb := keyboards.ApproveLineupKeyboard(teamID, gameID, game.Players, approvedSet)

	storedMsgID := approveLineupMessageID[userID]
	if storedMsgID > 0 {
		edit := tgbotapi.NewEditMessageText(chatID, storedMsgID, text)
		edit.ParseMode = tgbotapi.ModeMarkdown
		edit.ReplyMarkup = &kb
		res.AddEdit(&edit)
	} else {
		reply := tgbotapi.NewMessage(chatID, text)
		reply.ParseMode = tgbotapi.ModeMarkdown
		reply.ReplyMarkup = &kb
		res.AddTemporaryReply(&reply)
	}

	user_state.SetUserState(userID, chatID, fmt.Sprintf("approve_lineup:1:{\"ti\":\"%s\",\"gi\":\"%s\"}", teamID, gameID))
	return res, nil
}

func (s *approveLineupState) approvedList(game *domain.Training) []domain.Player {
	var out []domain.Player
	for _, p := range game.Players {
		if s.ApprovedIDs[p.ID] {
			out = append(out, p)
		}
	}
	return out
}

func approvedLineupMessage(game *domain.Training, approved []domain.Player) string {
	weekdays := []string{"Воскресенье", "Понедельник", "Вторник", "Среда", "Четверг", "Пятница", "Суббота"}

	// Шапка сообщения
	msg := "❗ Внимание ❗\n"
	if game.Division != "" {
		msg += "Игра по Дивизиону " + game.Division + "\n\n"
	} else {
		msg += "Игра на федерацию\n\n"
	}

	msg += "📅 " + weekdays[game.Date.Weekday()] + ", " + game.Date.Format("02.01") + "\n\n"
	msg += game.Type.ToString() + " против «" + game.Opponent + "»\n\n"
	msg += "⏰ " + game.Date.Format("15:04") + "\n"
	msg += "📍 " + game.Venue + "\n\n"
	msg += "Состав:\n\n"

	var goalkeepers []string
	var players []string
	fpc := 0
	for _, p := range approved {
		if p.Position == domain.PositionGoalkeeper {
			n := p.FullName
			if p.Commercial {
				n += " ₽₽₽"
			}
			goalkeepers = append(goalkeepers, "#вратарь "+n)
		} else {
			fpc++
			players = append(players, fmt.Sprintf("%d. %s", fpc, p.FullName))
		}
	}
	if len(goalkeepers) == 0 {
		goalkeepers = append(goalkeepers, "#вратарь")
	}

	msg += strings.Join(goalkeepers, "\n") + "\n\n"
	if len(players) > 0 {
		msg += strings.Join(players, "\n")
	} else {
		msg += "—"
	}

	return msg
}

func approveLineupStep2Confirm(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	res := NewAction()
	gameID := data.Get("game_id")
	if gameID == "" {
		gameID = data.Get("g")
	}
	if gameID == "" {
		gameID = data.Get("gi")
	}
	if gameID == "" {
		res.AddReply(&tgbotapi.MessageConfig{
			BaseChat: tgbotapi.BaseChat{ChatID: chatID},
			Text:     "Игра не указана.",
		})
		user_state.ClearUserState(userID, chatID)
		return res, nil
	}

	state, ok := approveLineupStateByUser[userID]
	if !ok {
		res.AddReply(&tgbotapi.MessageConfig{
			BaseChat: tgbotapi.BaseChat{ChatID: chatID},
			Text:     "Сессия утверждения состава истекла. Начните заново.",
		})
		user_state.ClearUserState(userID, chatID)
		return res, nil
	}

	game, err := st.GetTrainingByID(gameID)
	if err != nil || game == nil {
		res.AddReply(&tgbotapi.MessageConfig{
			BaseChat: tgbotapi.BaseChat{ChatID: chatID},
			Text:     "Игру не удалось загрузить.",
		})
		delete(approveLineupStateByUser, userID)
		delete(approveLineupMessageID, userID)
		user_state.ClearUserState(userID, chatID)
		return res, nil
	}

	approved := state.approvedList(game)

	// Минимум 5 полевых и 1 вратарь
	var fieldCount, gkCount int
	for _, p := range approved {
		if p.Position == domain.PositionGoalkeeper {
			gkCount++
		} else {
			fieldCount++
		}
	}
	if fieldCount < 5 || gkCount < 1 {
		res.CallbackAlert = "⚠️ Состав недостаточного размера: нужно минимум 5 полевых игроков и 1 вратарь."
		return res, nil
	}

	approvedPlayerIDs := make([]string, 0, len(approved))
	for _, p := range approved {
		approvedPlayerIDs = append(approvedPlayerIDs, p.ID)
	}
	err = st.SetApprovedPlayers(gameID, approvedPlayerIDs)
	if err != nil {
		res.AddReply(&tgbotapi.MessageConfig{
			BaseChat: tgbotapi.BaseChat{ChatID: chatID},
			Text:     "Не удалось сохранить состав.",
		})
		delete(approveLineupStateByUser, userID)
		delete(approveLineupMessageID, userID)
		user_state.ClearUserState(userID, chatID)
		return res, nil
	}

	// Удалить клавиатуру у сообщения тренеру
	targetMsgID := approveLineupMessageID[userID]
	if targetMsgID > 0 {
		edit := tgbotapi.NewEditMessageText(chatID, targetMsgID, approvedLineupMessage(game, approved))
		edit.ParseMode = tgbotapi.ModeMarkdown
		edit.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{}}
		res.AddEdit(&edit)
	}

	// Если в командном чате уже было сообщение с утверждённым составом — удаляем его
	if game.Team.ChatID != nil && *game.Team.ChatID != 0 && game.ApprovedMessageID != 0 {
		del := tgbotapi.NewDeleteMessage(*game.Team.ChatID, game.ApprovedMessageID)
		res.AddDelete(&del)
	}

	// Сообщение в командный чат с финальным составом (без клавиатуры), закрепляем его
	if game.Team.ChatID != nil && *game.Team.ChatID != 0 {
		teamMsg := tgbotapi.NewMessage(*game.Team.ChatID, approvedLineupMessage(game, approved))
		teamMsg.ParseMode = tgbotapi.ModeMarkdown
		res.AddPin(&teamMsg)
		// Попросим роутер сохранить message_id этого сообщения для данной тренировки
		res.ApprovedMessageTrainingID = game.ID
	}

	// Убираем все временные сообщения (в том числе клавиатуру выбора игры)
	res.FlushTemporary = true

	delete(approveLineupStateByUser, userID)
	delete(approveLineupMessageID, userID)
	user_state.ClearUserState(userID, chatID)
	return res, nil
}
