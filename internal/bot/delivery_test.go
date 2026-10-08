package bot

import (
	"bytes"
	"errors"
	"fmt"
	"hockey_bot/internal/bot/scenario"
	state "hockey_bot/internal/bot/user_state"
	"hockey_bot/internal/domain"
	"hockey_bot/internal/storage"
	"log"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestPublicationStoresReturnedMessageID(t *testing.T) {
	for _, approved := range []bool{false, true} {
		t.Run(fmt.Sprint("approved=", approved), func(t *testing.T) {
			c := newConversation(t)
			tr := c.training(c.team())
			action := scenario.NewAction()
			msg := tgbotapi.NewMessage(-100, "Состав")
			action.AddPin(&msg)
			if approved {
				action.ApprovedMessageTrainingID = tr.ID
			} else {
				action.TeamMessageTrainingID = tr.ID
			}
			executeAction(c.bot, c.store, 42, 42, action)
			calls := c.calls()
			if len(calls) != 2 || calls[0].method != "sendMessage" || calls[1].method != "pinChatMessage" {
				t.Fatalf("wrong publication sequence: %v", calls)
			}
			got, err := c.store.GetTrainingByID(tr.ID)
			if err != nil {
				t.Fatal(err)
			}
			id := got.TeamMessageID
			if approved {
				id = got.ApprovedMessageID
			}
			if id != calls[0].messageID || calls[1].values.Get("message_id") != fmt.Sprint(id) {
				t.Fatalf("wrong message ID: %d, calls=%v", id, calls)
			}
		})
	}
}

type metadataFailureStore struct{ storage.Storage }

func (s metadataFailureStore) UpdateTeamMessageID(string, int) error {
	return errors.New("metadata failure")
}
func (s metadataFailureStore) UpdateApprovedMessageID(string, int) error {
	return errors.New("metadata failure")
}

func TestPublicationFailuresAreObservable(t *testing.T) {
	for _, failure := range []string{"sendMessage", "pinChatMessage", "metadata"} {
		t.Run(failure, func(t *testing.T) {
			c := newConversation(t)
			tr := c.training(c.team())
			action := scenario.NewAction()
			msg := tgbotapi.NewMessage(-100, "Состав")
			action.AddPin(&msg)
			action.TeamMessageTrainingID = tr.ID
			c.api.respond = func(method string, v url.Values) (string, error) {
				if method == failure && v.Get("chat_id") == "-100" {
					return "", errors.New("injected publication failure")
				}
				return "", nil
			}
			var out bytes.Buffer
			old := log.Writer()
			log.SetOutput(&out)
			defer log.SetOutput(old)
			st := c.store
			if failure == "metadata" {
				st = metadataFailureStore{st}
			}
			executeAction(c.bot, st, 42, 42, action)
			if !strings.Contains(out.String(), "failure") {
				t.Fatalf("failure not logged: %s", out.String())
			}
			got, err := c.store.GetTrainingByID(tr.ID)
			if err != nil {
				t.Fatal(err)
			}
			if failure == "sendMessage" {
				if got.TeamMessageID != 500 {
					t.Fatal("failed send replaced message ID")
				}
				notified := false
				for _, call := range c.calls() {
					if call.method == "pinChatMessage" {
						t.Fatal("pinned failed message")
					}
					if call.method == "sendMessage" && call.values.Get("chat_id") == "42" && strings.Contains(call.values.Get("text"), "не удалось") {
						notified = true
					}
				}
				if !notified {
					t.Fatal("user not notified about failed publication")
				}
			}
			if failure == "pinChatMessage" && got.TeamMessageID == 500 {
				t.Fatal("lost delivered message ID after pin failed")
			}
		})
	}
}

func TestTemporaryMessagesAreDeletedOnlyFromTheirOwnChat(t *testing.T) {
	c := newConversation(t)
	action := scenario.NewAction()
	for _, chat := range []int64{42, -100} {
		m := tgbotapi.NewMessage(chat, "Вопрос")
		action.AddTemporaryReply(&m)
	}
	executeAction(c.bot, nil, 42, 42, action)
	sent := c.calls()
	flush := scenario.NewAction()
	flush.FlushTemporary = true
	executeAction(c.bot, nil, 42, 42, flush)
	calls := c.calls()
	if len(calls) != 3 || calls[2].method != "deleteMessage" || calls[2].values.Get("chat_id") != "42" || calls[2].values.Get("message_id") != fmt.Sprint(sent[0].messageID) {
		t.Fatalf("wrong deletion: %v", calls)
	}
	if len(state.DrainMessageBuffer(42, 42)) != 0 {
		t.Fatal("buffer retained deleted message")
	}
	group := state.DrainMessageBuffer(42, -100)
	if len(group) != 1 || group[0].MessageID != sent[1].messageID {
		t.Fatalf("other chat buffer was lost: %v", group)
	}
}

func TestConcurrentRosterDeliveryEndsWithLatestPlayers(t *testing.T) {
	c := newConversation(t)
	tr := c.training(c.team())
	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondStarted := make(chan struct{})
	secondDone := make(chan struct{})
	var releaseOnce sync.Once
	var mu sync.Mutex
	requests := 0
	visible := ""
	c.api.respond = func(method string, v url.Values) (string, error) {
		if method == "editMessageText" {
			mu.Lock()
			requests++
			first := requests == 1
			mu.Unlock()
			if first {
				close(firstEntered)
				<-releaseFirst
			}
			mu.Lock()
			visible = v.Get("text")
			mu.Unlock()
		}
		return "", nil
	}
	action := func() *scenario.Action {
		a := scenario.NewAction()
		a.RefreshTrainingID = tr.ID
		e := tgbotapi.NewEditMessageText(-100, 500, "old")
		a.AddEdit(&e)
		return a
	}
	var wg sync.WaitGroup
	t.Cleanup(func() { releaseOnce.Do(func() { close(releaseFirst) }); wg.Wait() })
	wg.Add(1)
	go func() { defer wg.Done(); executeAction(c.bot, c.store, 42, 42, action()) }()
	select {
	case <-firstEntered:
	case <-time.After(3 * time.Second):
		t.Fatal("first edit did not start")
	}
	p := domain.Player{ID: "new", TeamID: tr.TeamID, TelegramID: 43, FullName: "Новый участник"}
	if err := c.store.CreatePlayer(&p); err != nil {
		t.Fatal(err)
	}
	if err := c.store.AddTrainingPlayer(tr.ID, p.ID); err != nil {
		t.Fatal(err)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		close(secondStarted)
		executeAction(c.bot, c.store, 43, 43, action())
		close(secondDone)
	}()
	<-secondStarted
	select {
	case <-secondDone:
		t.Fatal("new roster overtook the blocked old edit and could be overwritten")
	case <-time.After(50 * time.Millisecond):
	}
	releaseOnce.Do(func() { close(releaseFirst) })
	wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(visible, p.FullName) {
		t.Fatalf("last delivered roster is stale: %s", visible)
	}
	edits := 0
	for _, call := range c.calls() {
		if call.method == "editMessageText" {
			edits++
		}
	}
	if edits != 2 {
		t.Fatalf("expected both updates to complete, got %d", edits)
	}
}
