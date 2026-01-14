// Точка входа бота с per-user goroutines для строгой последовательности
package main

import (
	"log"
	"os"
	"os/signal"
	"sync"

	"hockey_bot/internal/bot"
	"hockey_bot/internal/config"
	"hockey_bot/internal/storage"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// UserHandler - обработчик сообщений для одного пользователя
// Гарантирует строгую последовательность сообщений от одного юзера
type UserHandler struct {
	msgChan chan tgbotapi.Update
	wg      sync.WaitGroup
	bot     *tgbotapi.BotAPI
	store   storage.Storage
}

// run запускает обработку сообщений для пользователя в отдельной goroutine
func (h *UserHandler) run() {
	defer h.wg.Done()

	for update := range h.msgChan {
		// Передаем в общий обработчик bot/handlers.go
		bot.HandleUpdate(h.bot, update, h.store)
	}
}

var (
	userHandlers = make(map[int64]*UserHandler)
	mu           sync.RWMutex
)

// getOrCreateUserHandler возвращает или создает handler для пользователя
func getOrCreateUserHandler(bot *tgbotapi.BotAPI, chatID int64, store storage.Storage) *UserHandler {
	mu.Lock()
	defer mu.Unlock()

	if handler, exists := userHandlers[chatID]; exists {
		return handler
	}

	// Создаем новый handler
	handler := &UserHandler{
		msgChan: make(chan tgbotapi.Update, 100), // буфер на 100 сообщений
		bot:     bot,
		store:   store,
	}
	handler.wg.Add(1)
	userHandlers[chatID] = handler

	// Запускаем goroutine обработки
	go handler.run()

	log.Printf("Created user handler for %d", chatID)
	return handler
}

// handleUserUpdate отправляет update в канал пользователя (асинхронно)
func handleUserUpdate(bot *tgbotapi.BotAPI, update tgbotapi.Update, store storage.Storage) {
	chatID := getChatID(update)
	if chatID == 0 {
		return // игнорируем групповые чаты пока
	}

	handler := getOrCreateUserHandler(bot, chatID, store)
	select {
	case handler.msgChan <- update:
		// сообщение отправлено в канал
	default:
		log.Printf("User %d channel full, dropping update", chatID)
	}
}

// getChatID безопасно извлекает chatID из update
func getChatID(update tgbotapi.Update) int64 {
	if update.Message != nil {
		return update.Message.Chat.ID
	}
	if update.CallbackQuery != nil && update.CallbackQuery.Message != nil {
		return update.CallbackQuery.Message.Chat.ID
	}
	return 0
}

func main() {
	// 1. Загружаем конфигурацию
	cfg := config.New()

	// 2. Инициализируем хранилище (закроем при выходе)
	store := storage.NewGORM(cfg.DBPath)
	defer store.Close()

	// 3. Создаем бота
	botAPI, err := tgbotapi.NewBotAPI(cfg.TelegramAPIToken)
	if err != nil {
		log.Fatalf("error creating bot: %v", err)
	}

	botAPI.Debug = false
	log.Printf("Authorized on account %s", botAPI.Self.UserName)

	// 4. Настраиваем получение обновлений
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := botAPI.GetUpdatesChan(u)

	// 5. Graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, os.Kill)
	go func() {
		<-c
		log.Println("Shutting down...")

		// Закрываем все каналы пользователей
		mu.Lock()
		for chatID, handler := range userHandlers {
			close(handler.msgChan)
			handler.wg.Wait()
			delete(userHandlers, chatID)
		}
		mu.Unlock()

		os.Exit(0)
	}()

	// 6. Главный цикл - per-user обработка
	for update := range updates {
		// Асинхронно отправляем в handler пользователя
		go handleUserUpdate(botAPI, update, store)
	}
}
