package domain

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestTrainingAnnouncement(t *testing.T) {
	tr := Training{Date: time.Date(2030, 1, 1, 19, 30, 0, 0, time.UTC), Venue: "Арена", Type: TrainingGaming, Capacity: 2, Message: "Светлая форма", Players: []Player{{ID: "g", FullName: "Вратарь", Position: PositionGoalkeeper, Commercial: true}, {ID: "p", FullName: "Иванов", Position: PositionForward}}}
	want := "01.01\nВторник\nТренировка игровая\n\n⏰ 19:30\n📍 Арена\n\n#вратарь Вратарь ₽₽₽\n#вратарь\n\n1. Иванов\n2.\n\nСветлая форма"
	if got := tr.String(); got != want {
		t.Fatalf("unexpected announcement:\n%s", got)
	}
	tr.Type = TrainingMatch
	tr.Opponent = "Север"
	tr.Division = "Любители"
	got := tr.String()
	for _, part := range []string{"Игра по Дивизиону Любители", "против «Север»", "Вторник, 01.01", "⏰ 19:30", "1. Иванов", "#вратарь Вратарь ₽₽₽", "Светлая форма"} {
		if !strings.Contains(got, part) {
			t.Fatalf("missing %q in %s", part, got)
		}
	}
	tr.Players[0].Commercial = false
	if strings.Contains(tr.String(), "₽") {
		t.Fatal("non-commercial goalkeeper marked as paid")
	}
}
func TestPlayerRosterOperationsPreserveOrderAndAreIdempotent(t *testing.T) {
	a, b := Player{ID: "a", FullName: "Первый"}, Player{ID: "b", FullName: "Второй"}
	tr := Training{}
	tr.AddPlayer(&a)
	tr.AddPlayer(&b)
	tr.AddPlayer(&Player{ID: "a", FullName: "Дубликат"})
	if !reflect.DeepEqual(tr.Players, []Player{a, b}) {
		t.Fatalf("order or uniqueness broken: %v", tr.Players)
	}
	tr.RemovePlayer(&Player{ID: "absent"})
	tr.RemovePlayer(&a)
	tr.RemovePlayer(&a)
	if !reflect.DeepEqual(tr.Players, []Player{b}) {
		t.Fatalf("wrong removal: %v", tr.Players)
	}
}
func TestDisplayNames(t *testing.T) {
	for value, want := range map[PlayerPosition]string{PositionGoalkeeper: "Вр", PositionForward: "Нп", PositionDefender: "Зщ", PositionTrainer: "Тр", "bad": "?"} {
		if got := value.ToString(); got != want {
			t.Fatalf("position %q=%q", value, got)
		}
	}
	for value, want := range map[TrainingType]string{TrainingGaming: "Тренировка игровая", TrainingTechnical: "Тренировка", TrainingShooting: "Броски на сухом льду", TrainingChecking: "Подкатка", TrainingMatch: "Игра на федерацию в рамках регулярного первенства", "bad": "Неизвестно"} {
		if got := value.ToString(); got != want {
			t.Fatalf("training %q=%q", value, got)
		}
	}
}
