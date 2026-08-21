package keyboards

import (
	"fmt"
	"hockey_bot/internal/domain"
	"testing"
)

func TestPaginatedFriendsKeyboard(t *testing.T) {
	players := make([]domain.Player, 18)
	for i := range players {
		players[i] = domain.Player{ID: fmt.Sprintf("player-%d", i), FullName: fmt.Sprintf("Друг %d", i)}
	}

	keyboard := PaginatedFriendsKeyboard("frgo", "tr", 6, 7, 1, players)
	if got, want := len(keyboard.InlineKeyboard), FriendsPageSize+1; got != want {
		t.Fatalf("rows count = %d, want %d", got, want)
	}
	if got := keyboard.InlineKeyboard[0][0].Text; got != "Друг 8" {
		t.Fatalf("first friend on second page = %q, want %q", got, "Друг 8")
	}
	navigation := keyboard.InlineKeyboard[len(keyboard.InlineKeyboard)-1]
	if got, want := len(navigation), 3; got != want {
		t.Fatalf("navigation buttons = %d, want %d", got, want)
	}
	if got := *navigation[0].CallbackData; got != `frgo_tr:7:{"p":0}` {
		t.Fatalf("previous callback = %q", got)
	}
	if got := *navigation[2].CallbackData; got != `frgo_tr:7:{"p":2}` {
		t.Fatalf("next callback = %q", got)
	}
}
