package scenario

import (
	"errors"
	"hockey_bot/internal/bot/keyboards"
	"hockey_bot/internal/bot/user_state"
	"hockey_bot/internal/storage"
	"log"
	"os"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type AddBot struct {
	BaseScenario
}

var _ Scenario = (*AddBot)(nil)

func NewAddBot() *AddBot {
	return &AddBot{
		BaseScenario{
			id: "add_bot",
			steps: []Step{
				askTeamNameForChat,
				addChatIdToTeam,
			},
		},
	}
}

func askTeamNameForChat(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	res := NewAction()

	teams, err := st.GetTeamsWithoutChatByAdmin(userID)
	if err != nil {
		return nil, err
	}

	replyMsg := tgbotapi.NewMessage(chatID, "")

	if len(teams) > 0 {
		replyMsg.Text = "Меня добавили в чат!\n\n" +
			"Это чат какой команды?"
		replyMsg.ReplyMarkup = keyboards.AddChatIDForTeamKeyboard(teams)
	} else {
		replyMsg.Text = "Перед тем, как добавлять меня в чат, надо быть админом команды"
		res.LeaveChat = true
	}

	res.AddTemporaryReply(&replyMsg)

	return res, nil
}

func addChatIdToTeam(st storage.Storage, chatID int64, userID int64, messageID int, data Data) (*Action, error) {
	res := NewAction()
	teamID := data.Get("id")

	_, err := st.GetTeamByID(teamID)
	if err != nil {
		return nil, errors.New("team not found")
	}

	replyMsg := tgbotapi.NewMessage(chatID, "")
	replyMsg.ParseMode = tgbotapi.ModeMarkdown
	err = st.UpdateTeamChatID(teamID, chatID)
	if err != nil {
		replyMsg.Text = "Не удалось сохранить привязать команду к чату. Попробуй добавить меня еще раз."
		res.LeaveChat = true
	} else {
		replyMsg.Text = "Всё получилось! Теперь я буду скидывать сюда анонсы тренировок"
	}

	res.AddReply(&replyMsg)
	res.FlushTemporary = true
	user_state.ClearUserState(userID)

	return res, nil
}

// IsBotAddedToChat проверяет, что бот добавлен в чат
func IsBotAddedToChat(msg *tgbotapi.Message) bool {
	if len(msg.NewChatMembers) == 0 {
		return false
	}

	for _, member := range msg.NewChatMembers {
		// Проверяем, что добавили именно нашего бота
		if member.IsBot && member.UserName == os.Getenv("HOCKEY_BOT_USERNAME") {
			return true
		}
	}

	return false
}

// HandleBotAddedToChat - срабатывает при добавлении бота в чат
func HandleBotAddedToChat(bot *tgbotapi.BotAPI, chatID int64, userID int64, store storage.Storage) error {
	teams, err := store.GetTeamsWithoutChatByAdmin(userID)

	if err != nil {
		log.Printf("error getting teams: %v", err)
	}

	var welcomeMsg tgbotapi.MessageConfig

	if len(teams) > 0 {
		welcomeMsg = tgbotapi.NewMessage(chatID,
			"Тренер добавил меня в чат!\n\n"+
				"Выбери команду, которой принадлежит этот чат\n")
		welcomeMsg.ReplyMarkup = keyboards.AddChatIDForTeamKeyboard(teams)
	} else {
		welcomeMsg = tgbotapi.NewMessage(chatID,
			"Похоже, у тебя нет команды. Нужно создать новую команду или присоединитсья к существующей\n\n"+
				"Для этого напиши мне в личку – разберемся")
	}
	welcomeMsg.ParseMode = "Markdown"

	if _, err := bot.Send(welcomeMsg); err != nil {
		log.Printf("error sending welcome message: %v", err)
	}

	log.Printf("Bot added to chat %d", chatID)

	return nil
}
