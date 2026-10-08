package bot

import (
	"fmt"
	"hockey_bot/internal/domain"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func (c *conversation) game(team domain.Team, opponent string, field, keepers int) domain.Training {
	c.t.Helper()
	g := domain.Training{ID: uuid.NewString(), TeamID: team.ID, Team: team, Type: domain.TrainingMatch, Date: time.Now().AddDate(0, 0, 1), Opponent: opponent, Division: "Любители", Venue: "Арена"}
	for i := 0; i < field+keepers; i++ {
		p := domain.Player{ID: uuid.NewString(), TeamID: team.ID, TelegramID: int64(100 + i), FullName: fmt.Sprintf("%s-%d", opponent, i), Position: domain.PositionForward}
		if i < keepers {
			p.Position = domain.PositionGoalkeeper
		}
		if err := c.store.CreatePlayer(&p); err != nil {
			c.t.Fatal(err)
		}
		g.Players = append(g.Players, p)
	}
	if err := c.store.CreateTraining(&g); err != nil {
		c.t.Fatal(err)
	}
	return g
}
func TestLineupMinimumRosterAndToggle(t *testing.T) {
	for _, tc := range []struct {
		name           string
		field, keepers int
	}{{"no goalkeeper", 5, 0}, {"too few field players", 4, 1}, {"valid", 5, 1}} {
		t.Run(tc.name, func(t *testing.T) {
			c := newConversation(t)
			team := c.team()
			g := c.game(team, "Север", tc.field, tc.keepers)
			c.callback(fmt.Sprintf(`approve_lineup:0:{"ti":%q}`, team.ID), 10)
			c.click("Утвердить состав")
			approved, err := c.store.GetApprovedPlayerIDs(g.ID)
			if err != nil {
				t.Fatal(err)
			}
			if tc.name != "valid" {
				if len(approved) != 0 {
					t.Fatal("insufficient lineup was saved")
				}
				calls := c.calls()
				last := calls[len(calls)-1]
				if last.method != "answerCallbackQuery" || last.values.Get("show_alert") != "true" || !strings.Contains(last.values.Get("text"), "минимум 5") {
					t.Fatalf("missing roster alert: %+v", last)
				}
				return
			}
			if len(approved) != 6 {
				t.Fatalf("valid lineup not saved: %v", approved)
			}
			initial, err := c.store.GetTrainingByID(g.ID)
			if err != nil || initial.ApprovedMessageID == 0 {
				t.Fatal("approved publication ID missing")
			}
			c.callback(fmt.Sprintf(`approve_lineup:0:{"ti":%q}`, team.ID), 10)
			c.click("Север-0")
			c.click("Утвердить состав")
			unchanged, err := c.store.GetApprovedPlayerIDs(g.ID)
			if err != nil || len(unchanged) != 6 {
				t.Fatal("failed approval overwrote previous lineup")
			}
			c.click("Север-0")
			c.click("Утвердить состав")
			final, err := c.store.GetTrainingByID(g.ID)
			if err != nil || final.ApprovedMessageID == initial.ApprovedMessageID {
				t.Fatal("updated lineup not published")
			}
			deleted := false
			for _, call := range c.calls() {
				if call.method == "deleteMessage" && call.values.Get("chat_id") == "-100" && call.values.Get("message_id") == fmt.Sprint(initial.ApprovedMessageID) {
					deleted = true
				}
			}
			if !deleted {
				t.Fatal("old approved announcement not removed")
			}
		})
	}
}

func TestLineupMultipleGamesAndStaleConfirmation(t *testing.T) {
	c := newConversation(t)
	team := c.team()
	a := c.game(team, "Север", 5, 1)
	b := c.game(team, "Юг", 5, 1)
	c.callback(fmt.Sprintf(`approve_lineup:0:{"ti":%q}`, team.ID), 10)
	c.wantText("Выбери игру")
	c.click("Север")
	c.click("Север-0")
	// Выбор другого матча должен создать отдельную панель, не править старую.
	c.callback(fmt.Sprintf(`approve_lineup:1:{"g":%q}`, b.ID), 20)
	calls := c.calls()
	newPanel := false
	for i := len(calls) - 1; i >= 0; i-- {
		if calls[i].method == "sendMessage" || calls[i].method == "editMessageText" {
			newPanel = calls[i].method == "sendMessage" && strings.Contains(calls[i].values.Get("text"), "Юг")
			break
		}
	}
	if !newPanel {
		t.Fatal("switching games reused the previous lineup panel")
	}
	c.callback(fmt.Sprintf(`approve_lineup:2:{"g":%q}`, a.ID), 10)
	approved, err := c.store.GetApprovedPlayerIDs(a.ID)
	if err != nil || len(approved) != 0 {
		t.Fatal("stale confirmation approved another game")
	}
	c.callback(fmt.Sprintf(`approve_lineup:1:{"g":%q}`, b.ID), 20)
	c.click("Утвердить состав")
	approved, err = c.store.GetApprovedPlayerIDs(b.ID)
	if err != nil || len(approved) != 6 {
		t.Fatalf("selected game not approved: %v %v", approved, err)
	}
}

func TestLineupMissingExpiredAndEmpty(t *testing.T) {
	c := newConversation(t)
	team := c.team()
	c.callback(fmt.Sprintf(`approve_lineup:0:{"ti":%q}`, team.ID), 10)
	c.wantText("Нет предстоящих игр")
	game := c.game(team, "Пустой", 0, 0)
	c.callback(fmt.Sprintf(`approve_lineup:0:{"ti":%q}`, team.ID), 10)
	c.wantText("Пока не набралось")
	c.user = 999
	c.chat = 999
	c.callback(fmt.Sprintf(`approve_lineup:2:{"g":%q}`, game.ID), 10)
	c.wantText("Сессия утверждения состава истекла")
	c.user = 42
	c.chat = 42
	c.callback(`approve_lineup:1:{"g":"missing"}`, 10)
	c.wantText("Игру не удалось загрузить")
	training := c.training(team)
	c.callback(fmt.Sprintf(`approve_lineup:1:{"g":%q}`, training.ID), 10)
	c.wantText("Это не игра")
}
