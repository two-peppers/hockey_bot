package scenario

import (
	"fmt"
	"hockey_bot/internal/bot/keyboards"
	"hockey_bot/internal/bot/user_state"
	"hockey_bot/internal/domain"
	"hockey_bot/internal/storage"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type DelAdmin struct {
	BaseScenario
}

var _ Scenario = (*DelAdmin)(nil)

func NewDelAdmin() *DelAdmin {
	return &DelAdmin{
		BaseScenario{
			id: "del_admin",
			steps: []Step{
				askAdminNameForDel,
				delAdmin,
			},
		},
	}
}

func askAdminNameForDel(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	teamID := data.Get("id")
	if teamID == "" {
		return nil, fmt.Errorf("no team id found in data")
	}

	team, err := st.GetTeamByID(teamID)
	if err != nil {
		return nil, err
	}

	localTrainingID[userID] = teamID

	admins := make([]domain.Player, 0)
	for _, id := range team.Admins {
		player, err := st.GetPlayer(id, team.ID)
		if err != nil || player.TelegramID == userID {
			continue
		}
		admins = append(admins, *player)
	}

	replyMsg := tgbotapi.NewMessage(chatID, "")
	replyMsg.ParseMode = tgbotapi.ModeMarkdown

	if len(admins) == 0 {
		replyMsg.Text = "В команде нет админов, кроме тебя"
	} else {
		replyMsg.Text = "Кого лишаем прав?"
		replyMsg.ReplyMarkup = keyboards.FriendsKeyboard("del", "admin", 1, admins)
	}

	res := NewAction()
	res.AddReply(&replyMsg)

	return res, nil
}

func delAdmin(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	defer user_state.ClearUserState(userID, chatID)

	teamID, ok := localTrainingID[userID]
	if !ok {
		return nil, fmt.Errorf("no team id found in data")
	}

	team, err := st.GetTeamByID(teamID)
	if err != nil {
		return nil, err
	}

	playerID := data.Get("id")
	if playerID == "" {
		return nil, fmt.Errorf("no player id found in data")
	}

	player, err := st.GetPlayerByID(playerID)
	if err != nil {
		return nil, err
	}

	filteredAdmins := make([]int64, 0)
	for _, id := range team.Admins {
		if id == player.TelegramID {
			continue
		}
		filteredAdmins = append(filteredAdmins, id)
	}
	team.Admins = filteredAdmins

	replyMsg := tgbotapi.NewMessage(chatID, "")
	replyMsg.ParseMode = tgbotapi.ModeMarkdown

	err = st.SaveTeam(team)
	if err != nil {
		replyMsg.Text = "Не удалось удалить админа. Попробуй позже или обратись к разработчику бота."
	} else {
		replyMsg.Text = "Администратор удален!"
	}

	deleteMsg := tgbotapi.NewDeleteMessage(chatID, messageID)

	res := NewAction()
	res.AddReply(&replyMsg)
	res.AddDelete(&deleteMsg)

	return res, nil
}
