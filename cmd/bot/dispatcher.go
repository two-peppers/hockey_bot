package main

import (
	"context"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Единственный отправитель сохраняет порядок обновлений каждого чата.
// Переполненная очередь замедляет чтение, но не теряет сообщения.
func runUpdates(ctx context.Context, updates <-chan tgbotapi.Update, handle func(tgbotapi.Update)) {
	queues := make(map[int64]chan tgbotapi.Update)
	var workers sync.WaitGroup
	defer func() {
		for _, queue := range queues {
			close(queue)
		}
		workers.Wait()
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case update, ok := <-updates:
			if !ok {
				return
			}
			chatID := getChatID(update)
			if chatID == 0 {
				continue
			}
			queue := queues[chatID]
			if queue == nil {
				queue = make(chan tgbotapi.Update, 100)
				queues[chatID] = queue
				workers.Add(1)
				go func() {
					defer workers.Done()
					for update := range queue {
						handle(update)
					}
				}()
			}
			select {
			case queue <- update:
			case <-ctx.Done():
				return
			}
		}
	}
}

func getChatID(update tgbotapi.Update) int64 {
	if update.Message != nil && update.Message.Chat != nil {
		return update.Message.Chat.ID
	}
	if update.CallbackQuery != nil && update.CallbackQuery.Message != nil && update.CallbackQuery.Message.Chat != nil {
		return update.CallbackQuery.Message.Chat.ID
	}
	return 0
}
