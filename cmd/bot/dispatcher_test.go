package main

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestDispatchPreservesOrderWithoutDroppingUpdates(t *testing.T) {
	updates := make(chan tgbotapi.Update, 600)
	for i := 0; i < 300; i++ {
		for _, chatID := range []int64{42, -10} {
			updates <- tgbotapi.Update{UpdateID: i, Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: chatID}}}
		}
	}
	close(updates)
	var mu sync.Mutex
	got := make(map[int64][]int)
	runUpdates(context.Background(), updates, func(u tgbotapi.Update) {
		mu.Lock()
		defer mu.Unlock()
		got[getChatID(u)] = append(got[getChatID(u)], u.UpdateID)
	})
	want := make([]int, 300)
	for i := range want {
		want[i] = i
	}
	for _, id := range []int64{42, -10} {
		if !reflect.DeepEqual(got[id], want) {
			t.Fatalf("chat %d: lost or reordered updates: %v", id, got[id])
		}
	}
}

func TestDispatchDifferentChatsRunConcurrently(t *testing.T) {
	updates := make(chan tgbotapi.Update, 2)
	for _, id := range []int64{1, 2} {
		updates <- tgbotapi.Update{Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: id}}}
	}
	close(updates)
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	done := make(chan struct{})
	go func() {
		runUpdates(context.Background(), updates, func(tgbotapi.Update) { started <- struct{}{}; <-release })
		close(done)
	}()
	defer func() { close(release); <-done }()
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(3 * time.Second):
			t.Fatal("another chat was blocked")
		}
	}
}

func TestDispatcherCancellationDrainsAcceptedUpdates(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		updates := make(chan tgbotapi.Update, 150)
		for i := 0; i < 150; i++ {
			updates <- tgbotapi.Update{UpdateID: i, CallbackQuery: &tgbotapi.CallbackQuery{Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 42}}}}
		}
		gate := make(chan struct{})
		done := make(chan struct{})
		got := []int{}
		go func() {
			runUpdates(ctx, updates, func(u tgbotapi.Update) { <-gate; got = append(got, u.UpdateID) })
			close(done)
		}()
		synctest.Wait() // обработчик и заполненная очередь заблокированы
		cancel()
		synctest.Wait()
		select {
		case <-done:
			t.Fatal("dispatcher did not wait for accepted work")
		default:
		}
		close(gate)
		synctest.Wait()
		<-done
		if len(got) != 101 {
			t.Fatalf("accepted %d updates, want active update + 100 queued", len(got))
		}
		for i, id := range got {
			if i != id {
				t.Fatalf("shutdown reordered updates: %v", got)
			}
		}
	})
}

func TestDispatcherIgnoresUpdatesWithoutChat(t *testing.T) {
	updates := make(chan tgbotapi.Update, 4)
	for _, u := range []tgbotapi.Update{{}, {Message: &tgbotapi.Message{}}, {CallbackQuery: &tgbotapi.CallbackQuery{}}, {CallbackQuery: &tgbotapi.CallbackQuery{Message: &tgbotapi.Message{}}}} {
		updates <- u
	}
	close(updates)
	runUpdates(context.Background(), updates, func(tgbotapi.Update) { t.Error("dispatched an update without chat") })
}
