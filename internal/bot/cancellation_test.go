package bot

import (
	"fmt"
	state "hockey_bot/internal/bot/user_state"
	"hockey_bot/internal/domain"
	"hockey_bot/internal/pendingreply"
	"net/url"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func TestCancellationThroughMessagesAndTimeout(t *testing.T) {
	for _, mode := range []string{"reply", "timeout", "blocked DM"} {
		t.Run(mode, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				c := newConversation(t)
				team := c.team()
				team.Admins = []int64{1}
				if err := c.store.SaveTeam(&team); err != nil {
					t.Fatal(err)
				}
				tr := c.training(team)
				p := domain.Player{ID: "player", TeamID: team.ID, TelegramID: c.user, FullName: "Игрок"}
				if err := c.store.CreatePlayer(&p); err != nil {
					t.Fatal(err)
				}
				if err := c.store.AddTrainingPlayer(tr.ID, p.ID); err != nil {
					t.Fatal(err)
				}
				if mode == "blocked DM" {
					c.api.respond = func(method string, values url.Values) (string, error) {
						if method == "sendMessage" && values.Get("chat_id") == "42" {
							return `{"ok":false,"error_code":403,"description":"bot was blocked by the user"}`, nil
						}
						return "", nil
					}
				}
				c.chat = -100
				c.callback(fmt.Sprintf(`nogo_tr:0:{"id":%q}`, tr.ID), 500)
				synctest.Wait()
				got, err := c.store.GetTrainingByID(tr.ID)
				if err != nil || len(got.Players) != 0 {
					t.Fatalf("cancellation not saved: %+v %v", got, err)
				}
				if mode == "reply" {
					c.chat = 42
					c.send("Заболел")
					c.wantState("")
				}
				time.Sleep(10 * time.Minute)
				synctest.Wait()
				notifications := []string{}
				for _, call := range c.calls() {
					if call.method == "sendMessage" && call.values.Get("chat_id") == "1" {
						notifications = append(notifications, call.values.Get("text"))
					}
				}
				if len(notifications) != 1 {
					t.Fatalf("expected exactly one coach notification: %v", notifications)
				}
				want := map[string]string{"reply": "Заболел", "timeout": "не оставил комментарий", "blocked DM": "не смог написать"}[mode]
				if !strings.Contains(notifications[0], want) {
					t.Fatalf("wrong notification: %v", notifications)
				}
				if got := state.GetUserState(42, 42); got != "" {
					t.Fatalf("completed request left live dialog: %q", got)
				}
				pendingreply.Resolve(42, tr.ID)
			})
		})
	}
}

func TestLateReplyDoesNotNotifyCoachTwice(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := newConversation(t)
		team := c.team()
		team.Admins = []int64{1}
		if err := c.store.SaveTeam(&team); err != nil {
			t.Fatal(err)
		}
		tr := c.training(team)
		p := domain.Player{ID: "player", TeamID: team.ID, TelegramID: c.user}
		if err := c.store.CreatePlayer(&p); err != nil {
			t.Fatal(err)
		}
		if err := c.store.AddTrainingPlayer(tr.ID, p.ID); err != nil {
			t.Fatal(err)
		}
		c.callback(fmt.Sprintf(`nogo_tr:0:{"id":%q}`, tr.ID), 500)
		time.Sleep(10 * time.Minute)
		synctest.Wait()
		c.send("Поздний ответ")
		count := 0
		for _, call := range c.calls() {
			if call.method == "sendMessage" && call.values.Get("chat_id") == "1" {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("late reply caused %d notifications", count)
		}
	})
}
