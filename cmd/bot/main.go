package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"hockey_bot/internal/bot"
	"hockey_bot/internal/config"
	"hockey_bot/internal/storage"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg := config.New()
	store := storage.NewGORM(cfg.DBPath)
	defer store.Close()

	botAPI, err := tgbotapi.NewBotAPI(cfg.TelegramAPIToken)
	if err != nil {
		return err
	}
	log.Printf("Authorized on account %s", botAPI.Self.UserName)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := botAPI.GetUpdatesChan(u)
	defer botAPI.StopReceivingUpdates()

	runUpdates(ctx, updates, func(update tgbotapi.Update) {
		bot.HandleUpdate(botAPI, update, store)
	})
	return nil
}
