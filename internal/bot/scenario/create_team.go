package scenario

import (
	"fmt"
	"hockey_bot/internal/bot/keyboards"
	state "hockey_bot/internal/bot/user_state"
	"hockey_bot/internal/domain"
	"hockey_bot/internal/storage"
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
)

// CreateTeam сценарий создания новой команды
type CreateTeam struct {
	BaseScenario
}

var _ Scenario = (*CreateTeam)(nil)

func NewCreateTeam() *CreateTeam {
	return &CreateTeam{
		BaseScenario: BaseScenario{
			id: "create_team",
			steps: []Step{
				askTeamName,
				createTeamWithName,
			},
		},
	}
}

// askTeamName спрашивает название команды
func askTeamName(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	state.SetUserState(userID, chatID, fmt.Sprintf("create_team:%d", 1))

	edit := tgbotapi.NewEditMessageText(chatID, messageID, "📝 Как называется команда?")
	edit.ParseMode = tgbotapi.ModeMarkdown

	res := NewAction()
	res.AddEdit(&edit)

	return res, nil
}

// createTeamWithName создает команду по переданному названию
func createTeamWithName(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	defer state.ClearUserState(userID, chatID)

	name := data["message"]
	res := NewAction()

	if _, err := st.GetTeamByName(name); err != nil {
		return nil, err // ошибка БД
	}

	replyMsg := tgbotapi.NewMessage(chatID, "")
	replyMsg.ReplyToMessageID = messageID
	replyMsg.ParseMode = tgbotapi.ModeMarkdown

	if _, err := st.GetTeamByName(name); err != nil {
		replyMsg.Text = "❌ Команда с таким названием уже существует!"
		res.AddReply(&replyMsg)

		return res, nil
	}

	// Создаем команду
	team := &domain.Team{
		ID:     uuid.New().String(),
		Name:   name,
		Admins: []int64{userID},
	}

	if err := st.CreateTeam(team); err != nil {
		replyMsg.Text = "❌ Ошибка создания команды. Попробуйте еще раз."
		res.AddReply(&replyMsg)

		return res, err
	}

	// Успех!
	replyMsg.Text = "🎉 Поздравляю, команда создана!\n\n" +
		"Теперь добавь меня в чат команды, чтобы я мог узнавать своих игроков.\n\n" +
		"🏒 Команда: *" + name + "*\n" +
		"👤 Админ: вы\n" +
		"🆔 ID: `" + team.ID + "`\n\n" +
		"_Обязательно сделай меня администратором группы с правами " +
		"\"Удаление сообщений\" и \"Закрепление сообшений\"_\n\n" +
		"Желаю удачи на тренировках! ⛸️"
	replyMsg.ReplyMarkup = keyboards.MainMenuKeyboard() // возвращаемся в меню

	res.AddReply(&replyMsg)

	log.Printf("Team created: %s (ID: %s) by user %d", name, team.ID, chatID)
	return res, nil
}
