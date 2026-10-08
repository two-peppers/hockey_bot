package pendingreply

import (
	"errors"
	"fmt"
	"hockey_bot/internal/domain"
	"hockey_bot/internal/storage"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type trackerStore struct {
	storage.Storage
	team *domain.Team
	err  error
}

func (s trackerStore) GetTeamByID(string) (*domain.Team, error) { return s.team, s.err }

type requestLog struct {
	mu       sync.Mutex
	requests []string
	fail     bool
}

func (r *requestLog) Do(req *http.Request) (*http.Response, error) {
	if err := req.ParseForm(); err != nil {
		return nil, err
	}
	method := req.URL.Path[strings.LastIndex(req.URL.Path, "/")+1:]
	body := `{"ok":true,"result":true}`
	if method == "getMe" {
		body = `{"ok":true,"result":{"id":1,"is_bot":true}}`
	} else {
		r.mu.Lock()
		r.requests = append(r.requests, method+":"+req.Form.Get("chat_id")+":"+req.Form.Get("message_id")+":"+req.Form.Get("text"))
		r.mu.Unlock()
		if r.fail {
			return nil, errors.New("injected network failure")
		}
		if method == "sendMessage" {
			body = `{"ok":true,"result":{"message_id":1,"chat":{"id":1}}}`
		}
	}
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
}
func (r *requestLog) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.requests...)
}
func trackerBot(t *testing.T) (*tgbotapi.BotAPI, *requestLog) {
	t.Helper()
	r := &requestLog{}
	b, err := tgbotapi.NewBotAPIWithClient("token", "https://telegram.invalid/bot%s/%s", r)
	if err != nil {
		t.Fatal(err)
	}
	return b, r
}
func trackerContext() *PendingReplyContext {
	return &PendingReplyContext{UserID: 42, TrainingID: "training", TeamID: "team", PlayerFullName: "Иванов", QuestionChatID: 42, QuestionMessageID: 17}
}

func TestTimeoutAndReplyLifecycle(t *testing.T) {
	for _, reply := range []bool{false, true} {
		t.Run(fmt.Sprint("reply=", reply), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				bot, requests := trackerBot(t)
				st := trackerStore{team: &domain.Team{Admins: []int64{1, 2}}}
				Register(bot, st, trackerContext())
				synctest.Wait()
				time.Sleep(10*time.Minute - time.Nanosecond)
				synctest.Wait()
				if len(requests.snapshot()) != 0 {
					t.Fatal("timeout fired early")
				}
				if reply {
					Resolve(42, "training")
				}
				time.Sleep(time.Nanosecond)
				synctest.Wait()
				got := requests.snapshot()
				if reply {
					if len(got) != 0 {
						t.Fatalf("notified after reply: %v", got)
					}
					return
				}
				if len(got) != 3 || got[0] != "deleteMessage:42:17:" || !strings.Contains(got[1], "sendMessage:1::") || !strings.Contains(got[2], "sendMessage:2::") || !strings.Contains(got[1], "Иванов") {
					t.Fatalf("wrong timeout notification: %v", got)
				}
				time.Sleep(20 * time.Minute)
				synctest.Wait()
				if len(requests.snapshot()) != 3 {
					t.Fatal("timeout delivered twice")
				}
			})
		})
	}
}

func TestReplacingPendingReplyCancelsOldDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bot, requests := trackerBot(t)
		st := trackerStore{team: &domain.Team{Admins: []int64{1}}}
		Register(bot, st, trackerContext())
		synctest.Wait()
		time.Sleep(5 * time.Minute)
		newer := trackerContext()
		newer.QuestionMessageID = 18
		Register(bot, st, newer)
		synctest.Wait()
		time.Sleep(5 * time.Minute)
		synctest.Wait()
		if len(requests.snapshot()) != 0 {
			t.Fatal("old request timed out")
		}
		time.Sleep(5 * time.Minute)
		synctest.Wait()
		got := requests.snapshot()
		if len(got) != 2 || got[0] != "deleteMessage:42:18:" {
			t.Fatalf("wrong request expired: %v", got)
		}
	})
}

func TestTimeoutFailuresAndNameFallback(t *testing.T) {
	for _, tc := range []struct {
		name     string
		team     *domain.Team
		err      error
		fail     bool
		username string
	}{
		{name: "missing team"}, {name: "database failure", err: errors.New("database unavailable")},
		{name: "delivery failure", team: &domain.Team{Admins: []int64{1, 2}}, fail: true},
		{name: "username", team: &domain.Team{Admins: []int64{1}}, username: "nickname"},
		{name: "numeric ID", team: &domain.Team{Admins: []int64{1}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				bot, requests := trackerBot(t)
				requests.fail = tc.fail
				ctx := trackerContext()
				ctx.PlayerFullName = ""
				ctx.Username = tc.username
				Register(bot, trackerStore{team: tc.team, err: tc.err}, ctx)
				synctest.Wait()
				time.Sleep(10 * time.Minute)
				synctest.Wait()
				got := requests.snapshot()
				want := 1
				if tc.team != nil {
					want += len(tc.team.Admins)
				}
				if len(got) != want {
					t.Fatalf("one failure prevented subsequent notifications: %v", got)
				}
				if tc.team != nil && !tc.fail {
					name := tc.username
					if name == "" {
						name = "Игрок 42"
					}
					if !strings.Contains(got[1], name) {
						t.Fatalf("wrong fallback: %v", got)
					}
				}
			})
		})
	}
}

func TestReplyAndTimeoutHaveSingleWinner(t *testing.T) {
	for i := 0; i < 20; i++ {
		synctest.Test(t, func(t *testing.T) {
			bot, requests := trackerBot(t)
			Register(bot, trackerStore{team: &domain.Team{Admins: []int64{1}}}, trackerContext())
			resolved := make(chan bool, 1)
			go func() { time.Sleep(10 * time.Minute); resolved <- Resolve(42, "training") }()
			time.Sleep(10 * time.Minute)
			synctest.Wait()
			won := <-resolved
			notifications := 0
			for _, r := range requests.snapshot() {
				if strings.HasPrefix(r, "sendMessage") {
					notifications++
				}
			}
			if won {
				notifications++
			}
			if notifications != 1 {
				t.Fatalf("reply won=%t, requests=%v", won, requests.snapshot())
			}
		})
	}
}

func TestOldReplyCannotResolveReplacement(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bot, requests := trackerBot(t)
		st := trackerStore{team: &domain.Team{Admins: []int64{1}}}
		old := trackerContext()
		old.RequestID = "old"
		Register(bot, st, old)
		newer := trackerContext()
		newer.RequestID = "new"
		Register(bot, st, newer)
		if ResolveRequest(42, "training", "old") {
			t.Fatal("stale reply resolved newer request")
		}
		if !ResolveRequest(42, "training", "new") {
			t.Fatal("current reply was rejected")
		}
		if ResolveRequest(42, "training", "new") {
			t.Fatal("duplicate reply accepted")
		}
		time.Sleep(10 * time.Minute)
		synctest.Wait()
		if len(requests.snapshot()) != 0 {
			t.Fatal("resolved request timed out")
		}
	})
}
