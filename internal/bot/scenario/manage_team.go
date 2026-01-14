package scenario

import (
	"errors"
	"fmt"
	"hockey_bot/internal/bot/keyboards"
	"hockey_bot/internal/bot/user_state"
	"hockey_bot/internal/storage"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// ManageTeam меню управления командой и выбора конмады, если их несколько
type ManageTeam struct {
	BaseScenario
}

var _ Scenario = (*ManageTeam)(nil)

func NewManageTeam() *ManageTeam {
	return &ManageTeam{
		BaseScenario: BaseScenario{
			id: "manage_team",
			steps: []Step{
				showTeams,
				manageOneTeam,
			},
		},
	}
}

func showTeams(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	res := NewAction()

	deleteMsg := tgbotapi.NewDeleteMessage(chatID, messageID)
	teams, err := st.GetTeamsByAdmin(chatID)

	if err != nil {
		return nil, err
	}

	reply := tgbotapi.NewMessage(chatID, "")

	fmt.Println("Teams len:", len(teams))

	if len(teams) == 0 {
		reply.Text = "У тебя ещё нет своей команды. Для начала надо её создать."
		user_state.ClearUserState(userID, chatID)
	} else if len(teams) == 1 {
		data = map[string]string{
			"team_id": teams[0].ID,
		}
		return manageOneTeam(st, chatID, userID, messageID, data)
	} else {
		reply.Text = "Выбери команду:"
		reply.ReplyMarkup = keyboards.AdminTeamKeyboard(teams)
	}

	reply.ParseMode = tgbotapi.ModeMarkdown

	res.AddReply(&reply)
	res.AddDelete(&deleteMsg)

	return res, nil
}

func manageOneTeam(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	deleteMsg := tgbotapi.NewDeleteMessage(chatID, messageID)

	teamID, ok := data["team_id"]
	if !ok {
		return nil, errors.New("no team_id in callback data")
	}

	team, err := st.GetTeamByID(teamID)
	if err != nil {
		return nil, err
	}

	reply := tgbotapi.NewMessage(chatID, fmt.Sprintf("Утравление командой *%s*", team.Name))
	reply.ParseMode = tgbotapi.ModeMarkdown
	reply.ReplyMarkup = keyboards.TeamManagementKeyboard(*team)

	res := NewAction()
	res.AddReply(&reply)
	res.AddDelete(&deleteMsg)

	return res, nil
}
