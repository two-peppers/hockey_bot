package scenario

import (
	"errors"
	"fmt"
	"hockey_bot/internal/bot/user_state"
	"hockey_bot/internal/storage"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type PlayersList struct {
	BaseScenario
}

var _ Scenario = (*PlayersList)(nil)

func NewPlayersList() *PlayersList {
	return &PlayersList{
		BaseScenario{
			id: "players",
			steps: []Step{
				showPlayersList,
			},
		},
	}
}

func showPlayersList(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	teamID := data.Get("id")
	if teamID == "" {
		return nil, errors.New("no team id")
	}

	team, err := requireTeamAdmin(st, teamID, userID)
	if err != nil {
		return nil, err
	}

	players, err := st.GetPlayersByTeamID(team.ID)
	if err != nil {
		return nil, err
	}

	replyMsg := tgbotapi.NewMessage(chatID, "")
	replyMsg.ParseMode = tgbotapi.ModeMarkdown

	if len(players) == 0 {
		replyMsg.Text = "Игроков пока нет. Чтобы тут что-то появилось, нужно, чтобы кто-то записался на тренировку"
	} else {
		replyMsg.Text = fmt.Sprintf("Игроки команды %s\n\n", team.Name)

		for i, player := range players {
			replyMsg.Text += fmt.Sprintf("%d. %s %s\n", i+1, player.FullName, player.Position.ToString())
		}
	}

	deleteMsg := tgbotapi.NewDeleteMessage(chatID, messageID)

	res := NewAction()

	res.AddReply(&replyMsg)
	res.AddDelete(&deleteMsg)
	user_state.ClearUserState(userID, chatID)

	return res, nil
}
