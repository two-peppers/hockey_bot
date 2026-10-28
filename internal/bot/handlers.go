// Обработчики команд и callback'ов
package bot

import (
	"hockey_bot/internal/bot/keyboards"
	"hockey_bot/internal/bot/scenario"
	"hockey_bot/internal/bot/user_state"
	"hockey_bot/internal/storage"
	"log"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// HandleUpdate - главный обработчик всех обновлений от Telegram
// Разделяет логику: команды /start, callback от кнопок
func HandleUpdate(bot *tgbotapi.BotAPI, update tgbotapi.Update, store storage.Storage) {
	// Игнорируем невалидные обновления
	if update.Message == nil && update.CallbackQuery == nil {
		return
	}

	// Обработка обычных сообщений (команды)
	if update.Message != nil {
		handleMessage(bot, *update.Message, store)
		return
	}

	// Обработка нажатий кнопок (callback query)
	if update.CallbackQuery != nil {
		handleCallback(bot, *update.CallbackQuery, store)
		return
	}
}

// handleMessage обрабатывает текстовые сообщения
func handleMessage(bot *tgbotapi.BotAPI, msg tgbotapi.Message, store storage.Storage) {
	if msg.Chat == nil || msg.From == nil {
		return
	}
	chatID := msg.Chat.ID
	senderID := msg.From.ID
	messageID := msg.MessageID
	text := msg.Text
	log.Printf("Message from %d in chat %d: %s", senderID, chatID, text)

	// Бота добавили в чат
	if scenario.IsBotAddedToChat(&msg) {
		user_state.SetUserState(senderID, chatID, "add_chat:0")
	}

	if msg.MigrateToChatID != 0 {
		log.Printf("Migrate From chat ID %d to chat ID: %d", chatID, msg.MigrateToChatID)
		newChatID := msg.MigrateToChatID

		team, err := store.GetTeamByChatID(chatID)
		if err == nil && team != nil {
			if err := store.UpdateTeamChatID(team.ID, newChatID); err != nil {
				log.Printf("failed to migrate team chat: %v", err)
			}
		} else if err != nil {
			log.Printf("failed to load team for chat migration: %v", err)
		}
	}

	// Только команда /start пока
	if strings.HasPrefix(text, "/start") {
		if chatID < 0 {
			delMsg := tgbotapi.NewDeleteMessage(chatID, messageID)
			requestLogged(bot, delMsg, "delete command")
			return
		}
		msg := tgbotapi.NewMessage(chatID, "Рад тебя видеть! Это главное меню бота\n\nВыбери действие:")
		msg.ReplyMarkup = keyboards.MainMenuKeyboard()
		if _, err := bot.Send(msg); err != nil {
			log.Printf("error sending message: %v", err)
		}
		user_state.ClearUserState(senderID, chatID)
		return
	}

	if strings.HasPrefix(text, "/trainings") {
		if chatID < 0 {
			delMsg := tgbotapi.NewDeleteMessage(chatID, messageID)
			requestLogged(bot, delMsg, "delete command")
			return
		}
		user_state.SetUserState(senderID, chatID, "trainings:0")
	}

	userName := msg.From.UserName
	if len(userName) == 0 {
		userName = msg.From.FirstName
	}
	if len(userName) == 0 {
		userName = msg.From.LastName
	}
	if len(userName) == 0 {
		userName = "Друг"
	}

	executeRoute(bot, store, chatID, senderID, messageID, userName, text)
}

// handleCallback обрабатывает нажатия inline кнопок
func handleCallback(bot *tgbotapi.BotAPI, callback tgbotapi.CallbackQuery, store storage.Storage) {
	if callback.Message == nil || callback.Message.Chat == nil || callback.From == nil {
		return
	}
	chatID := callback.Message.Chat.ID
	senderID := callback.From.ID
	data := callback.Data
	messageID := callback.Message.MessageID

	log.Printf("Callback from %d in chat %d: %s", senderID, chatID, data)

	if _, _, _, err := parseRoute(data); err != nil {
		log.Printf("invalid callback from user %d: %v", senderID, err)
		requestLogged(bot, tgbotapi.CallbackConfig{CallbackQueryID: callback.ID, Text: "Кнопка устарела. Открой меню заново.", ShowAlert: true}, "answer invalid callback")
		return
	}
	user_state.SetUserState(senderID, chatID, data)

	userName := callback.From.UserName
	if len(userName) == 0 {
		userName = callback.From.FirstName
	}
	if len(userName) == 0 {
		userName = callback.From.LastName
	}
	if len(userName) == 0 {
		userName = "Друг"
	}

	action := executeRoute(bot, store, chatID, senderID, messageID, userName, "")

	if action != nil && action.CallbackAlert != "" {
		cfg := tgbotapi.CallbackConfig{CallbackQueryID: callback.ID, Text: action.CallbackAlert, ShowAlert: true}
		if _, err := bot.Request(cfg); err != nil {
			log.Printf("error answering callback with alert: %v", err)
		}
	} else {
		answer := tgbotapi.NewCallback(callback.ID, "")
		if _, err := bot.Request(answer); err != nil {
			log.Printf("error answering callback: %v", err)
		}
	}
}
