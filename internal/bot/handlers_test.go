package bot

import (
	"fmt"
	"hockey_bot/internal/domain"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
)

func TestTeamBindingMigrationAndAdminMenus(t *testing.T) {
	c := newConversation(t)
	t.Setenv("HOCKEY_BOT_USERNAME", "test_bot")
	team := domain.Team{ID: uuid.NewString(), Name: "Север", Admins: []int64{c.user}}
	other := domain.Team{ID: uuid.NewString(), Name: "Юг", Admins: []int64{c.user}}
	for _, tm := range []*domain.Team{&team, &other} {
		if err := c.store.CreateTeam(tm); err != nil {
			t.Fatal(err)
		}
	}
	c.send("/start")
	c.click("Управлять командой")
	c.click("Север")
	c.wantText("Север")
	c.click("Запланировать тренировку")
	c.wantText("добавить бота")
	c.chat = -100
	HandleUpdate(c.bot, tgbotapi.Update{Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: c.chat}, From: &tgbotapi.User{ID: c.user}, NewChatMembers: []tgbotapi.User{{IsBot: true, UserName: "test_bot"}}}}, c.store)
	c.click("Север")
	c.wantText("Всё получилось")
	got, err := c.store.GetTeamByID(team.ID)
	if err != nil || got.ChatID == nil || *got.ChatID != -100 {
		t.Fatal("team chat not bound")
	}
	HandleUpdate(c.bot, tgbotapi.Update{Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: -100}, From: &tgbotapi.User{ID: c.user}, MigrateToChatID: -200}}, c.store)
	got, err = c.store.GetTeamByID(team.ID)
	if err != nil || got.ChatID == nil || *got.ChatID != -200 {
		t.Fatal("supergroup migration not saved")
	}
	player := domain.Player{ID: uuid.NewString(), TeamID: team.ID, TelegramID: 77, FullName: "Петров", Position: domain.PositionDefender}
	if err := c.store.CreatePlayer(&player); err != nil {
		t.Fatal(err)
	}
	c.chat = 42
	c.send("/start")
	c.click("Управлять командой")
	c.click("Север")
	c.click("Список игроков")
	c.wantText("Петров Зщ")
	c.callback(fmt.Sprintf(`set_admin:0:{"id":%q}`, team.ID), 10)
	c.click("Петров")
	c.wantText("Администратор добавлен")
	got, err = c.store.GetTeamByID(team.ID)
	if err != nil || len(got.Admins) != 2 {
		t.Fatal("admin not added")
	}
	c.callback(fmt.Sprintf(`del_admin:0:{"id":%q}`, team.ID), 10)
	c.click("Петров")
	c.wantText("Администратор удален")
	got, err = c.store.GetTeamByID(team.ID)
	if err != nil || len(got.Admins) != 1 || got.Admins[0] != 42 {
		t.Fatal("wrong admin removed")
	}
}
func TestUpcomingTrainingCommandsAndInvalidUpdates(t *testing.T) {
	c := newConversation(t)
	for _, u := range []tgbotapi.Update{{}, {Message: &tgbotapi.Message{}}, {CallbackQuery: &tgbotapi.CallbackQuery{}}} {
		HandleUpdate(c.bot, u, c.store)
	}
	if len(c.calls()) != 0 {
		t.Fatal("invalid update caused request")
	}
	c.send("/trainings")
	c.wantText("Предстоящих тренировок нет")
	team := c.team()
	tr := c.training(team)
	p := domain.Player{ID: uuid.NewString(), TeamID: team.ID, TelegramID: 42, FullName: "Иванов"}
	if err := c.store.CreatePlayer(&p); err != nil {
		t.Fatal(err)
	}
	c.send("/trainings")
	c.click("Команда")
	c.wantText(tr.Date.Format("15:04"))
	c.chat = -100
	before := len(c.calls())
	c.send("/start")
	c.send("/trainings")
	after := c.calls()[before:]
	if len(after) != 2 || after[0].method != "deleteMessage" || after[1].method != "deleteMessage" {
		t.Fatalf("commands leaked into group: %v", after)
	}
}
