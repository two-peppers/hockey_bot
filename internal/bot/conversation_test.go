package bot

import (
	"encoding/json"
	"fmt"
	state "hockey_bot/internal/bot/user_state"
	"hockey_bot/internal/domain"
	"hockey_bot/internal/storage"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
)

// Диалоги проходят через публичный обработчик. Кнопки извлекаем из ответа
// бота, чтобы обнаруживать несовместимость клавиатур, роутера и сценариев.
type conversation struct {
	t           *testing.T
	bot         *tgbotapi.BotAPI
	api         *fakeTelegram
	store       storage.Storage
	user, chat  int64
	nextMessage int
}

func newConversation(t *testing.T) *conversation {
	t.Helper()
	b, api := newFakeBot(t)
	st := storage.NewGORM(filepath.Join(t.TempDir(), "bot.db"))
	t.Cleanup(st.Close)
	c := &conversation{t: t, bot: b, api: api, store: st, user: 42, chat: 42, nextMessage: 1000}
	t.Cleanup(func() {
		for _, chat := range []int64{42, -100, -200} {
			state.ClearUserState(42, chat)
			state.DrainMessageBuffer(42, chat)
		}
	})
	return c
}
func (c *conversation) send(text string) {
	c.t.Helper()
	c.nextMessage++
	HandleUpdate(c.bot, tgbotapi.Update{Message: &tgbotapi.Message{MessageID: c.nextMessage, Chat: &tgbotapi.Chat{ID: c.chat}, From: &tgbotapi.User{ID: c.user, FirstName: "Тест"}, Text: text}}, c.store)
	c.validateKeyboards()
}
func (c *conversation) callback(data string, id int) {
	c.t.Helper()
	HandleUpdate(c.bot, tgbotapi.Update{CallbackQuery: &tgbotapi.CallbackQuery{ID: fmt.Sprint(c.nextMessage), From: &tgbotapi.User{ID: c.user, FirstName: "Тест"}, Message: &tgbotapi.Message{MessageID: id, Chat: &tgbotapi.Chat{ID: c.chat}}, Data: data}}, c.store)
	c.validateKeyboards()
}
func (c *conversation) calls() []telegramCall {
	c.api.mu.Lock()
	defer c.api.mu.Unlock()
	return append([]telegramCall(nil), c.api.calls...)
}
func (c *conversation) click(label string) {
	c.t.Helper()
	calls := c.calls()
	for i := len(calls) - 1; i >= 0; i-- {
		call := calls[i]
		if call.values.Get("chat_id") != strconv.FormatInt(c.chat, 10) {
			continue
		}
		var kb tgbotapi.InlineKeyboardMarkup
		if json.Unmarshal([]byte(call.values.Get("reply_markup")), &kb) != nil {
			continue
		}
		for _, row := range kb.InlineKeyboard {
			for _, button := range row {
				if strings.Contains(button.Text, label) && button.CallbackData != nil {
					id := call.messageID
					if call.method == "editMessageText" {
						id, _ = strconv.Atoi(call.values.Get("message_id"))
					}
					c.callback(*button.CallbackData, id)
					return
				}
			}
		}
	}
	c.t.Fatalf("button %q not found in chat %d", label, c.chat)
}
func (c *conversation) validateKeyboards() {
	c.t.Helper()
	for _, call := range c.calls() {
		var kb tgbotapi.InlineKeyboardMarkup
		if json.Unmarshal([]byte(call.values.Get("reply_markup")), &kb) != nil {
			continue
		}
		for _, row := range kb.InlineKeyboard {
			for _, b := range row {
				if b.CallbackData != nil && (len(*b.CallbackData) < 1 || len(*b.CallbackData) > 64) {
					c.t.Fatalf("invalid Telegram callback size %d: %s", len(*b.CallbackData), *b.CallbackData)
				}
			}
		}
	}
}
func (c *conversation) wantText(text string) {
	c.t.Helper()
	calls := c.calls()
	for i := len(calls) - 1; i >= 0; i-- {
		if calls[i].values.Get("chat_id") == strconv.FormatInt(c.chat, 10) && (calls[i].method == "sendMessage" || calls[i].method == "editMessageText") {
			if !strings.Contains(calls[i].values.Get("text"), text) {
				c.t.Fatalf("want last response containing %q, got %q", text, calls[i].values.Get("text"))
			}
			return
		}
	}
	c.t.Fatal("no response")
}
func (c *conversation) wantState(prefix string) {
	c.t.Helper()
	s := state.GetUserState(c.user, c.chat)
	if (prefix == "" && s != "") || (prefix != "" && !strings.HasPrefix(s, prefix)) {
		c.t.Fatalf("state = %q, want %q", s, prefix)
	}
}
func (c *conversation) team() domain.Team {
	c.t.Helper()
	chatID := int64(-100)
	team := domain.Team{ID: uuid.NewString(), Name: "Команда", Admins: []int64{c.user}, ChatID: &chatID}
	if err := c.store.CreateTeam(&team); err != nil {
		c.t.Fatal(err)
	}
	return team
}
func (c *conversation) training(team domain.Team) domain.Training {
	c.t.Helper()
	tr := domain.Training{ID: uuid.NewString(), TeamID: team.ID, Team: team, Date: time.Now().AddDate(0, 0, 1), Type: domain.TrainingGaming, Capacity: 10, Venue: "Арена", TeamMessageID: 500}
	if err := c.store.CreateTraining(&tr); err != nil {
		c.t.Fatal(err)
	}
	return tr
}
func (c *conversation) publishedTraining() *domain.Training {
	c.t.Helper()
	for _, call := range c.calls() {
		if call.method != "sendMessage" || call.values.Get("chat_id") != "-100" {
			continue
		}
		var kb tgbotapi.InlineKeyboardMarkup
		if json.Unmarshal([]byte(call.values.Get("reply_markup")), &kb) != nil || len(kb.InlineKeyboard) == 0 {
			continue
		}
		raw := *kb.InlineKeyboard[0][0].CallbackData
		_, _, data, err := parseRoute(raw)
		if err != nil {
			c.t.Fatal(err)
		}
		tr, err := c.store.GetTrainingByID(data.Get("id"))
		if err != nil || tr == nil {
			c.t.Fatalf("published training missing: %v", err)
		}
		if tr.TeamMessageID != call.messageID {
			c.t.Fatalf("message ID not saved: %d != %d", tr.TeamMessageID, call.messageID)
		}
		return tr
	}
	c.t.Fatal("no training published to group")
	return nil
}

func TestTrainingAndGameDialogs(t *testing.T) {
	for _, game := range []bool{false, true} {
		t.Run(fmt.Sprint("game=", game), func(t *testing.T) {
			c := newConversation(t)
			team := c.team()
			// Недавняя арена должна появиться в подсказках.
			c.training(team)
			c.send("/start")
			c.click("Управлять командой")
			if game {
				c.click("Запланировать игру")
			} else {
				c.click("Запланировать тренировку")
			}
			c.wantText("Когда")
			c.send("31.02.2030")
			c.wantText("Неверный формат даты")
			c.send("01.01.2030")
			c.wantText("Во сколько")
			for _, bad := range []string{"25:00", "12:60", "abc:15", "12:xx", "12", ":", "-1:00"} {
				c.send(bad)
				c.wantText("Неверный формат времени")
			}
			c.send("19:30")
			c.wantText("арене")
			c.send("Арена")
			if game {
				c.send(strings.Repeat("я", 33))
				c.wantText("Максимум 32")
				c.send("Любители")
				c.send(strings.Repeat("я", 65))
				c.wantText("Попробуй еще раз")
				c.send("Соперник")
			} else {
				c.click("Техническая")
				for _, bad := range []string{"много", "0", "-1"} {
					c.send(bad)
					c.wantState("train:11")
				}
				c.send("12")
				c.send("Берите светлую форму")
			}
			c.wantState("")
			tr := c.publishedTraining()
			want := time.Date(2030, 1, 1, 19, 30, 0, 0, time.Local)
			if !tr.Date.Equal(want) || tr.Venue != "Арена" || tr.TeamID != team.ID {
				t.Fatalf("wrong event: %+v; date want %v", tr, want)
			}
			if game {
				if tr.Type != domain.TrainingMatch || tr.Opponent != "Соперник" || tr.Division != "Любители" {
					t.Fatalf("wrong game: %+v", tr)
				}
			} else if tr.Type != domain.TrainingTechnical || tr.Capacity != 12 || tr.Message != "Берите светлую форму" {
				t.Fatalf("wrong training: %+v", tr)
			}
			pinned := false
			for _, call := range c.calls() {
				if call.method == "pinChatMessage" && call.values.Get("message_id") == fmt.Sprint(tr.TeamMessageID) {
					pinned = true
				}
			}
			if !pinned {
				t.Fatal("announcement not pinned")
			}
		})
	}
}

func TestRegistrationAndFriendDialogs(t *testing.T) {
	for _, position := range []string{"Нападающий", "Защитник", "Вратарь"} {
		wantPosition := map[string]domain.PlayerPosition{"Нападающий": domain.PositionForward, "Защитник": domain.PositionDefender, "Вратарь": domain.PositionGoalkeeper}[position]
		for _, commercial := range []bool{false, true} {
			if position != "Вратарь" && commercial {
				continue
			}
			t.Run(fmt.Sprintf("%s/paid=%t", position, commercial), func(t *testing.T) {
				c := newConversation(t)
				tr := c.training(c.team())
				c.chat = -100
				c.callback(fmt.Sprintf(`go_tr:0:{"id":%q}`, tr.ID), 500)
				c.send(strings.Repeat("я", 33))
				c.wantText("очень длинная")
				c.send("Иванов")
				c.click(position)
				if position == "Вратарь" {
					if commercial {
						c.click("Да")
					} else {
						c.click("Нет")
					}
				}
				c.wantState("")
				player, err := c.store.GetPlayer(c.user, tr.TeamID)
				if err != nil || player == nil || player.FullName != "Иванов" || player.Commercial != commercial || player.Position != wantPosition {
					t.Fatalf("wrong player: %+v %v", player, err)
				}
				c.callback(fmt.Sprintf(`go_tr:0:{"id":%q}`, tr.ID), 500)
				got, err := c.store.GetTrainingByID(tr.ID)
				if err != nil || len(got.Players) != 1 {
					t.Fatalf("repeat signup duplicated player: %+v %v", got, err)
				}
				// Новый друг проходит тот же путь, но остаётся отдельным участником.
				c.callback(fmt.Sprintf(`frgo_tr:0:{"id":%q}`, tr.ID), 500)
				c.send("Петров")
				c.click(position)
				if position == "Вратарь" {
					if commercial {
						c.click("Да")
					} else {
						c.click("Нет")
					}
				}
				got, err = c.store.GetTrainingByID(tr.ID)
				if err != nil || len(got.Players) != 2 || got.Players[1].FullName != "Петров" || got.Players[1].Commercial != commercial || got.Players[1].Position != wantPosition {
					t.Fatalf("friend missing: %+v %v", got, err)
				}
				friendID := got.Players[1].ID
				c.callback(fmt.Sprintf(`frnogo_tr:0:{"id":%q}`, tr.ID), 500)
				c.click("Петров")
				got, err = c.store.GetTrainingByID(tr.ID)
				if err != nil || len(got.Players) != 1 || got.Players[0].ID != player.ID {
					t.Fatalf("cancelled wrong player: %+v %v", got, err)
				}
				c.callback(fmt.Sprintf(`frgo_tr:0:{"id":%q}`, tr.ID), 500)
				c.click("Петров")
				got, err = c.store.GetTrainingByID(tr.ID)
				if err != nil || len(got.Players) != 2 || got.Players[1].ID != friendID {
					t.Fatalf("existing friend not reused: %+v %v", got, err)
				}
			})
		}
	}
}

func TestTeamCreationAndDuplicateName(t *testing.T) {
	c := newConversation(t)
	c.send("/start")
	c.click("Создать команду")
	c.send("Север")
	team, err := c.store.GetTeamByName("Север")
	if err != nil || team == nil || len(team.Admins) != 1 || team.Admins[0] != c.user {
		t.Fatalf("team not created: %+v %v", team, err)
	}
	c.send("/start")
	c.click("Создать команду")
	c.send("Север")
	c.wantText("уже существует")
	got, err := c.store.GetTeamByName("Север")
	if err != nil || got.ID != team.ID {
		t.Fatal("duplicate changed original team")
	}
}

func TestCancelFriendPaginationOnlyListsOwnedEnrolledFriends(t *testing.T) {
	c := newConversation(t)
	team := c.team()
	tr := c.training(team)
	c.chat = -100
	owner := domain.Player{ID: uuid.NewString(), TeamID: team.ID, TelegramID: 42, FullName: "Владелец"}
	if err := c.store.CreatePlayer(&owner); err != nil {
		t.Fatal(err)
	}
	if err := c.store.AddTrainingPlayer(tr.ID, owner.ID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		friend := domain.Player{ID: uuid.NewString(), TeamID: team.ID, TelegramID: 42, FullName: fmt.Sprintf("Друг %02d", i)}
		if err := c.store.CreatePlayer(&friend); err != nil {
			t.Fatal(err)
		}
		if err := c.store.AddTrainingPlayer(tr.ID, friend.ID); err != nil {
			t.Fatal(err)
		}
	}
	foreign := domain.Player{ID: uuid.NewString(), TeamID: team.ID, TelegramID: 99, FullName: "Чужой друг"}
	if err := c.store.CreatePlayer(&foreign); err != nil {
		t.Fatal(err)
	}
	if err := c.store.AddTrainingPlayer(tr.ID, foreign.ID); err != nil {
		t.Fatal(err)
	}
	c.callback(fmt.Sprintf(`frnogo_tr:0:{"id":%q}`, tr.ID), 500)
	c.click("Вперёд")
	c.click("Друг 09")
	got, err := c.store.GetTrainingByID(tr.ID)
	if err != nil || len(got.Players) != 11 {
		t.Fatalf("wrong roster after paginated cancellation: %+v %v", got, err)
	}
	for _, p := range got.Players {
		if p.FullName == "Друг 09" {
			t.Fatal("selected friend still enrolled")
		}
	}
	c.callback(fmt.Sprintf(`frnogo_tr:0:{"id":%q}`, tr.ID), 500)
	c.callback(fmt.Sprintf(`frnogo_tr:1:{"id":%q}`, foreign.ID), 500)
	got, err = c.store.GetTrainingByID(tr.ID)
	if err != nil || len(got.Players) != 11 {
		t.Fatal("forged callback removed another user's player")
	}
	c.wantText("недостаточно прав")
}
