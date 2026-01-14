package config

import (
	"log"
	"os"
)

type Config struct {
	TelegramAPIToken string
	DBPath           string
	BotName          string
}

func New() *Config {
	token := os.Getenv("TELEGRAM_APITOKEN")
	if token == "" {
		log.Fatal("TELEGRAM_APITOKEN environment variable required")
	}

	dbPath := os.Getenv("HOCKEY_BOT_DB_PATH")
	if dbPath == "" {
		dbPath = "./hockey_bot.db"
	}

	botName := os.Getenv("HOCKEY_BOT_USERNAME")
	if botName == "" {
		log.Fatal("HOCKEY_BOT_USERNAME environment variable required")
	}

	return &Config{
		TelegramAPIToken: token,
		DBPath:           dbPath,
		BotName:          botName,
	}
}
