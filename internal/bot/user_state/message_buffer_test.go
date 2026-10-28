package user_state

import (
	"sync"
	"testing"
)

func TestMessageBufferConcurrentChatsAndDrain(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) { defer wg.Done(); AddToMessageBuffer(int64(i%2+1), 42, i) }(i)
	}
	wg.Wait()
	for _, chatID := range []int64{1, 2} {
		messages := DrainMessageBuffer(42, chatID)
		if len(messages) != 50 {
			t.Fatalf("chat %d: got %d messages", chatID, len(messages))
		}
		for _, msg := range messages {
			if msg.ChatID != chatID {
				t.Fatal("messages leaked between chats")
			}
		}
		if len(DrainMessageBuffer(42, chatID)) != 0 {
			t.Fatal("buffer was not cleared")
		}
	}
}

func TestFinishingOldRequestPreservesNewDialog(t *testing.T) {
	SetUserState(42, 42, "new request")
	defer ClearUserState(42, 42)
	if ClearUserStateIf(42, 42, "old request") {
		t.Fatal("cleared unrelated dialog")
	}
	if got := GetUserState(42, 42); got != "new request" {
		t.Fatalf("new dialog lost: %q", got)
	}
	if !ClearUserStateIf(42, 42, "new request") || GetUserState(42, 42) != "" {
		t.Fatal("matching request not cleared")
	}
}
