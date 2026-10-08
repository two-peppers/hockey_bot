package scenario

import (
	"testing"
	"time"
)

func TestEventDatesUseLocalCalendar(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	for _, base := range []time.Time{
		time.Date(2026, 12, 31, 23, 50, 0, 0, time.FixedZone("MSK", 3*3600)),
		time.Date(2026, 3, 29, 0, 30, 0, 0, berlin),
	} {
		for text, days := range map[string]int{"Сегодня": 0, "Завтра": 1, "Послезавтра": 2, "+3 дня": 3} {
			got, err := parseEventDate(text, base)
			want := time.Date(base.Year(), base.Month(), base.Day()+days, 0, 0, 0, 0, base.Location())
			if err != nil || !got.Equal(want) || got.Location() != base.Location() {
				t.Fatalf("%s from %v: got %v, want %v (%v)", text, base, got, want, err)
			}
		}
		got, err := parseEventDate("01.01.2027", base)
		want := time.Date(2027, 1, 1, 0, 0, 0, 0, base.Location())
		if err != nil || !got.Equal(want) {
			t.Fatalf("explicit date got %v, want %v (%v)", got, want, err)
		}
	}
	for _, input := range []string{"31.02.2027", "29.02.2027", "", "tomorrow"} {
		if _, err := parseEventDate(input, time.Now()); err == nil {
			t.Fatalf("accepted invalid date %q", input)
		}
	}
}
