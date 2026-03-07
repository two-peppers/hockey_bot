package keyboards

import (
	"fmt"
	"hockey_bot/internal/domain"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// MainMenuKeyboard возвращает главное меню с кнопками
func MainMenuKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		// Первая строка - "Создать команду"
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🏒 Создать команду", "create_team:0"),
		),
		// Вторая строка - "Предстоящие тренировки"
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📅 Предстоящие тренировки", "trainings:0"),
		),
		// Третья строка - "Управлять командой"
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⚙️ Управлять командой", "manage_team:0"),
		),
	)
}

// AddChatIDForTeamKeyboard возвращает клавиатуру с командами для привязки к чату
func AddChatIDForTeamKeyboard(teams []domain.Team) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton

	for _, t := range teams {
		button := tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(t.Name, fmt.Sprintf("add_chat:1:{\"id\": \"%s\"}", t.ID)),
		)
		rows = append(rows, button)
	}

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

func TeamJoinKeyboard(teams []domain.Team) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton

	for _, t := range teams {
		button := tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(t.Name, "join_team:"+t.ID),
		)
		rows = append(rows, button)
	}

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

func PlayerPositionKeyboard(userState string) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⚔️ Нападающий", fmt.Sprintf("%s:{\"position\": \"%s\"}", userState, domain.PositionForward)),
		),
		// Вторая строка - "Присоединиться к команде"
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🛡️ Защитник", fmt.Sprintf("%s:{\"position\": \"%s\"}", userState, domain.PositionDefender)),
		),
		// Третья строка - "Управлять командой"
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🥅 Вратарь", fmt.Sprintf("%s:{\"position\": \"%s\"}", userState, domain.PositionGoalkeeper)),
		),
	)
}

// AdminTeamKeyboard - выбор команды для управления
func AdminTeamKeyboard(teams []domain.Team) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, team := range teams {
		data := fmt.Sprintf("manage_team:1:{\"team_id\":\"%s\"}", team.ID)
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(team.Name, data),
		))
	}
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// TeamManagementKeyboard - меню управления командой
func TeamManagementKeyboard(team domain.Team) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📅 Запланировать тренировку", fmt.Sprintf("train:0:{\"team_id\":\"%s\"}", team.ID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⚽ Запланировать игру", fmt.Sprintf("game:0:{\"team_id\":\"%s\"}", team.ID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Утвердить состав на игру", fmt.Sprintf("approve_lineup:0:{\"ti\":\"%s\"}", team.ID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📋 Список игроков", fmt.Sprintf("players:0:{\"id\":\"%s\"}", team.ID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("👑 Назначить админа", fmt.Sprintf("set_admin:0:{\"id\":\"%s\"}", team.ID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔻 Разжаловать админа", fmt.Sprintf("del_admin:0:{\"id\":\"%s\"}", team.ID)),
		),
		//tgbotapi.NewInlineKeyboardRow(
		//	tgbotapi.NewInlineKeyboardButtonData("◀️ Назад", "back_to_main"),
		//),
	)
}

// QuickDateReplyKeyboard - клавиатура ВМЕСТО поля ввода
func QuickDateReplyKeyboard() tgbotapi.ReplyKeyboardMarkup {
	return tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("Сегодня"),
			tgbotapi.NewKeyboardButton("Завтра"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("Послезавтра"),
			tgbotapi.NewKeyboardButton("+3 дня"),
		),
	)
}

func TrainingTypeKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⚽ Игровая", "train:9:{\"type\":\"gaming\"}"),
			tgbotapi.NewInlineKeyboardButtonData("🔧 Техническая", "train:9:{\"type\":\"technical\"}"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🎯 Бросковая", "train:9:{\"type\":\"shooting\"}"),
			tgbotapi.NewInlineKeyboardButtonData("⛸️ Подкатка", "train:9:{\"type\":\"checking\"}"),
		),
	)
}

func TrainingCapacityKeyboard() tgbotapi.ReplyKeyboardMarkup {
	return tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("5"),
			tgbotapi.NewKeyboardButton("10"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("20"),
			tgbotapi.NewKeyboardButton("25"),
		),
	)
}

func TrainingNoMessageKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Не нужно", "train:13"),
		),
	)
}

func TakePartKeyboard(eventType string, eventId string) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Пойду", fmt.Sprintf("go_%s:0:{\"id\":\"%s\"}", eventType, eventId)),
			tgbotapi.NewInlineKeyboardButtonData("❌ Не пойду", fmt.Sprintf("nogo_%s:0:{\"id\":\"%s\"}", eventType, eventId)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Друг пойдет", fmt.Sprintf("frgo_%s:0:{\"id\":\"%s\"}", eventType, eventId)),
			tgbotapi.NewInlineKeyboardButtonData("❌ Друг не пойдет", fmt.Sprintf("frnogo_%s:0:{\"id\":\"%s\"}", eventType, eventId)),
		),
	)
}

func VenueKeyboard(venues []string) tgbotapi.ReplyKeyboardMarkup {
	// Максимум 4 кнопки в строке (UX)
	const cols = 2

	var rows [][]tgbotapi.KeyboardButton
	for i := 0; i < len(venues); i += cols {
		end := i + cols
		if end > len(venues) {
			end = len(venues)
		}

		row := make([]tgbotapi.KeyboardButton, 0, cols)
		for _, venue := range venues[i:end] {
			row = append(row, tgbotapi.NewKeyboardButton(venue))
		}
		rows = append(rows, row)
	}

	return tgbotapi.NewReplyKeyboard(rows...)
}

func FriendsKeyboard(prefix string, eventType string, step int, players []domain.Player) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton

	for _, player := range players {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(player.FullName, fmt.Sprintf("%s_%s:%d:{\"id\":\"%s\"}", prefix, eventType, step, player.ID))),
		)
	}

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

func CommercialGoalkeeperKeyboard(prefix string, eventType string) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Да", fmt.Sprintf("%s_%s:5:{\"comm\": \"true\"}", prefix, eventType)),
			tgbotapi.NewInlineKeyboardButtonData("❌ Нет", fmt.Sprintf("%s_%s:5:{\"comm\": \"false\"}", prefix, eventType)),
		),
	)
}

// ApproveLineupGameChoiceKeyboard — выбор игры при нескольких предстоящих (callback до 64 байт).
func ApproveLineupGameChoiceKeyboard(teamID string, games []domain.Training) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, g := range games {
		label := g.Date.Format("02.01") + " «" + g.Opponent + "»"
		data := fmt.Sprintf("approve_lineup:1:{\"g\":\"%s\"}", g.ID)
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(label, data),
		))
	}
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

func TrainingsChoiceKeyboard(trainings []domain.Training) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton

	for _, tr := range trainings {
		label := fmt.Sprintf("%s %s • %s • %s", tr.Date.Format("02.01"), tr.Date.Format("15:04"), tr.Team.Name, tr.Type.ToString())
		data := fmt.Sprintf("trainings:1:{\"id\":\"%s\"}", tr.ID)
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(label, data),
		))
	}

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// ApproveLineupKeyboard — игроки (желающие) в два столбца с галочкой/крестиком, затем кнопка «Утвердить состав» на всю ширину.
// approvedSet — множество ID утверждённых. Callback до 64 байт: только g (game_id) и t (player_id).
func ApproveLineupKeyboard(teamID, gameID string, willing []domain.Player, approvedSet map[string]bool) tgbotapi.InlineKeyboardMarkup {
	const cols = 2
	var rows [][]tgbotapi.InlineKeyboardButton

	for i := 0; i < len(willing); i += cols {
		var row []tgbotapi.InlineKeyboardButton
		for j := 0; j < cols && i+j < len(willing); j++ {
			p := willing[i+j]
			in := approvedSet[p.ID]
			icon := "❌"
			if in {
				icon = "✅"
			}
			label := icon + " " + p.FullName
			cb := fmt.Sprintf("approve_lineup:1:{\"g\":\"%s\",\"t\":\"%s\"}", gameID, p.ID)
			if len(cb) > 64 {
				cb = fmt.Sprintf("approve_lineup:1:{\"t\":\"%s\"}", p.ID)
			}
			row = append(row, tgbotapi.NewInlineKeyboardButtonData(label, cb))
		}
		if len(row) == 1 {
			// Нечётное количество: справа пустая кнопка-заглушка (ничего не делает при нажатии)
			row = append(row, tgbotapi.NewInlineKeyboardButtonData(" ", "approve_lineup:1:{}"))
		}
		rows = append(rows, row)
	}

	// Кнопка «Утвердить состав» — на всю ширину (отдельная строка)
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("🚀 Утвердить состав", fmt.Sprintf("approve_lineup:2:{\"g\":\"%s\"}", gameID)),
	))
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}
