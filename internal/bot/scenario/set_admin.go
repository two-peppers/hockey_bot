package scenario

import (
	"fmt"
	"hockey_bot/internal/bot/keyboards"
	"hockey_bot/internal/bot/user_state"
	"hockey_bot/internal/domain"
	"hockey_bot/internal/storage"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type SetAdmin struct {
	BaseScenario
}

var _ Scenario = (*SetAdmin)(nil)

var localTrainingID = make(map[sessionKey]string)

func NewSetAdmin() *SetAdmin {
	return &SetAdmin{
		BaseScenario{
			id: "set_admin",
			steps: []Step{
				askAdminNameForSet,
				setAdmin,
			},
		},
	}
}

func askAdminNameForSet(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	teamID := data.Get("id")
	if teamID == "" {
		return nil, fmt.Errorf("no team id found in data")
	}

	team, err := requireTeamAdmin(st, teamID, userID)
	if err != nil {
		return nil, err
	}

	localTrainingID[sessionKey{userID, chatID}] = teamID

	players, err := st.GetPlayersByTeamID(team.ID)
	if err != nil {
		return nil, err
	}

	filteredPlayers := make([]domain.Player, 0)
	for _, p := range players {
		if p.TelegramID != userID {
			filteredPlayers = append(filteredPlayers, p)
		}
	}

	players = filteredPlayers

	replyMsg := tgbotapi.NewMessage(chatID, "Кто будет админом команды?")
	replyMsg.ReplyMarkup = keyboards.FriendsKeyboard("set", "admin", 1, players)
	replyMsg.ParseMode = tgbotapi.ModeMarkdown

	res := NewAction()
	res.AddReply(&replyMsg)

	return res, nil
}

func setAdmin(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	defer user_state.ClearUserState(userID, chatID)

	teamID, ok := localTrainingID[sessionKey{userID, chatID}]
	if !ok {
		return nil, fmt.Errorf("no team id found in data")
	}

	team, err := requireTeamAdmin(st, teamID, userID)
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

	if player == nil {
		return nil, ErrNotFound
	}
	if player.TeamID != team.ID {
		return nil, ErrForbidden
	}
	team.Admins = append(team.Admins, player.TelegramID)

	replyMsg := tgbotapi.NewMessage(chatID, "")
	replyMsg.ParseMode = tgbotapi.ModeMarkdown

	err = st.SaveTeam(team)
	if err != nil {
		replyMsg.Text = "Не удалось добавить админа. Попробуй позже или обратись к разработчику бота."
	} else {
		replyMsg.Text = "Администратор добавлен!"
	}

	deleteMsg := tgbotapi.NewDeleteMessage(chatID, messageID)

	res := NewAction()
	res.AddReply(&replyMsg)
	res.AddDelete(&deleteMsg)

	return res, err
}
