package bot

import (
	"bytes"
	"errors"
	"fmt"
	"hockey_bot/internal/bot/keyboards"
	"hockey_bot/internal/bot/scenario"
	state "hockey_bot/internal/bot/user_state"
	"hockey_bot/internal/domain"
	"hockey_bot/internal/storage"
	"io"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type telegramCall struct {
	method    string
	values    url.Values
	messageID int
}
type fakeTelegram struct {
	mu         sync.Mutex
	calls      []telegramCall
	failMethod string
	failure    string
	respond    func(string, url.Values) (string, error)
}

func (f *fakeTelegram) Do(req *http.Request) (*http.Response, error) {
	if err := req.ParseForm(); err != nil {
		return nil, err
	}
	method := req.URL.Path[strings.LastIndex(req.URL.Path, "/")+1:]
	f.mu.Lock()
	messageID := 123 + len(f.calls)
	f.calls = append(f.calls, telegramCall{method: method, values: req.Form, messageID: messageID})
	f.mu.Unlock()
	if f.respond != nil {
		body, err := f.respond(method, req.Form)
		if err != nil {
			return nil, err
		}
		if body != "" {
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
		}
	}
	body := `{"ok":true,"result":true}`
	if method == "getMe" {
		body = `{"ok":true,"result":{"id":1,"is_bot":true,"username":"test"}}`
	}
	if method == "sendMessage" || method == "editMessageText" {
		body = fmt.Sprintf(`{"ok":true,"result":{"message_id":%d,"chat":{"id":%s}}}`, messageID, req.Form.Get("chat_id"))
	}
	if method == f.failMethod {
		body = fmt.Sprintf(`{"ok":false,"error_code":400,"description":%q}`, f.failure)
	}
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
}
func newFakeBot(t *testing.T) (*tgbotapi.BotAPI, *fakeTelegram) {
	t.Helper()
	client := &fakeTelegram{}
	bot, err := tgbotapi.NewBotAPIWithClient("token", "https://telegram.invalid/bot%s/%s", client)
	if err != nil {
		t.Fatal(err)
	}
	client.calls = nil
	return bot, client
}

func TestPaginationCallbacksRoundTripThroughRouter(t *testing.T) {
	players := make([]domain.Player, 18)
	for i := range players {
		players[i] = domain.Player{ID: fmt.Sprint(i), FullName: "Друг"}
	}
	for _, spec := range []struct {
		prefix               string
		selectStep, pageStep int
	}{{"frgo", 6, 7}, {"frnogo", 1, 2}} {
		keyboard := keyboards.PaginatedFriendsKeyboard(spec.prefix, "tr", spec.selectStep, spec.pageStep, 1, players)
		nav := keyboard.InlineKeyboard[len(keyboard.InlineKeyboard)-1]
		for i, button := range nav {
			sc, step, data, err := parseRoute(*button.CallbackData)
			if err != nil || sc != spec.prefix+"_tr" || step != spec.pageStep || data.Get("p") != fmt.Sprint(i) {
				t.Fatalf("invalid generated callback %q: %v", *button.CallbackData, err)
			}
		}
		_, _, data, err := parseRoute(fmt.Sprintf(`%s_tr:%d:{"p":2}`, spec.prefix, spec.pageStep))
		if err != nil || data.Get("p") != "2" {
			t.Fatalf("old numeric callback rejected: %v", err)
		}
	}
}

func TestMalformedCallbacksDoNotPanicOrReplaceCurrentDialog(t *testing.T) {
	bot, client := newFakeBot(t)
	for _, data := range []string{"garbage", "go_tr:-1", "go_tr:999", "go_tr:x", `go_tr:0:null`, `go_tr:0:{"id":null}`, `go_tr:0:{"id":{}}`, `go_tr:0:{`, `go_tr:0:[]`} {
		state.SetUserState(42, 42, "train:1")
		handleCallback(bot, tgbotapi.CallbackQuery{ID: "cb", From: &tgbotapi.User{ID: 42}, Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 42}}, Data: data}, nil)
		if state.GetUserState(42, 42) != "train:1" {
			t.Fatalf("invalid callback %q replaced dialog", data)
		}
		last := client.calls[len(client.calls)-1]
		if last.method != "answerCallbackQuery" || last.values.Get("show_alert") != "true" {
			t.Fatal("missing invalid callback alert")
		}
	}
	state.ClearUserState(42, 42)
}

func TestFriendPaginationRunsThroughHandler(t *testing.T) {
	st := storage.NewGORM(filepath.Join(t.TempDir(), "bot.db"))
	t.Cleanup(st.Close)
	team := domain.Team{ID: "team", Name: "Команда"}
	if err := st.CreateTeam(&team); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 11; i++ {
		p := domain.Player{ID: fmt.Sprint(i), TeamID: team.ID, TelegramID: 42, FullName: fmt.Sprintf("Друг %d", i)}
		if err := st.CreatePlayer(&p); err != nil {
			t.Fatal(err)
		}
	}
	training := domain.Training{ID: "training", TeamID: team.ID}
	if err := st.CreateTraining(&training); err != nil {
		t.Fatal(err)
	}
	bot, client := newFakeBot(t)
	cb := tgbotapi.CallbackQuery{ID: "cb", From: &tgbotapi.User{ID: 42}, Message: &tgbotapi.Message{MessageID: 5, Chat: &tgbotapi.Chat{ID: -100}}, Data: `frgo_tr:0:{"id":"training"}`}
	handleCallback(bot, cb, st)
	cb.Message.MessageID = 123
	for _, data := range []string{`frgo_tr:7:{"p":1}`, `frgo_tr:7:{"p":"0"}`} {
		before := len(client.calls)
		cb.Data = data
		handleCallback(bot, cb, st)
		edited := false
		for _, call := range client.calls[before:] {
			if call.method == "editMessageText" && call.values.Get("reply_markup") != "" {
				edited = true
			}
		}
		if !edited {
			t.Fatalf("page was not updated: %q", data)
		}
	}
	state.ClearUserState(42, -100)
	state.DrainMessageBuffer(42, -100)
}

type routeStore struct {
	storage.Storage
	training *domain.Training
	failure  error
}

func (s routeStore) GetTrainingByID(string) (*domain.Training, error) { return s.training, s.failure }
func (s routeStore) GetUpcomingTrainingsByTelegramID(int64, int) ([]domain.Training, error) {
	return nil, s.failure
}

func TestScenarioErrorIsLoggedAndReported(t *testing.T) {
	bot, client := newFakeBot(t)
	var output bytes.Buffer
	old := log.Writer()
	log.SetOutput(&output)
	defer log.SetOutput(old)
	state.SetUserState(42, 42, "trainings:0")
	defer state.ClearUserState(42, 42)
	executeRoute(bot, routeStore{failure: errors.New("database unavailable")}, 42, 42, 0, "Игрок", "")
	if !strings.Contains(output.String(), "database unavailable") {
		t.Fatal("storage error was not logged")
	}
	if len(client.calls) != 1 || client.calls[0].method != "sendMessage" || !strings.Contains(client.calls[0].values.Get("text"), "Не удалось") {
		t.Fatalf("user not informed: %+v", client.calls)
	}
}

func TestTelegramEditErrorsAreLoggedAndReported(t *testing.T) {
	for _, description := range []string{"message to edit not found", "message is not modified"} {
		t.Run(description, func(t *testing.T) {
			bot, client := newFakeBot(t)
			client.failMethod = "editMessageText"
			client.failure = description
			var output bytes.Buffer
			old := log.Writer()
			log.SetOutput(&output)
			defer log.SetOutput(old)
			action := scenario.NewAction()
			edit := tgbotapi.NewEditMessageText(42, 10, "new")
			action.AddEdit(&edit)
			executeAction(bot, nil, 42, 42, action)
			if description == "message is not modified" {
				if len(client.calls) != 1 {
					t.Fatal("reported harmless unchanged message")
				}
			} else if len(client.calls) != 2 || client.calls[1].method != "sendMessage" || !strings.Contains(output.String(), description) {
				t.Fatalf("edit error was hidden: %+v %s", client.calls, output.String())
			}
		})
	}
}

func TestTrainingDisplayReloadsCurrentRosterAndSendsEditsInOrder(t *testing.T) {
	bot, client := newFakeBot(t)
	current := &domain.Training{ID: "training", Players: []domain.Player{{ID: "new", FullName: "Новый игрок"}}}
	st := routeStore{training: current}
	for i := 0; i < 2; i++ {
		action := scenario.NewAction()
		action.RefreshTrainingID = current.ID
		for _, messageID := range []int{10, 11} {
			edit := tgbotapi.NewEditMessageText(-100, messageID, "устаревший состав")
			action.AddEdit(&edit)
		}
		executeAction(bot, st, 42, 42, action)
	}
	if len(client.calls) != 4 {
		t.Fatalf("edits have not completed: %+v", client.calls)
	}
	for i, call := range client.calls {
		if call.values.Get("message_id") != fmt.Sprint(10+i%2) || call.values.Get("text") != current.String() {
			t.Fatalf("stale or reordered edit: %+v", call)
		}
	}
}
