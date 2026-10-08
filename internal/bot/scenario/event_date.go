package scenario

import "time"

// Дни календаря и введённые даты относятся к часовому поясу бота.
func parseEventDate(text string, now time.Time) (time.Time, error) {
	offsets := map[string]int{"Сегодня": 0, "Завтра": 1, "Послезавтра": 2, "+3 дня": 3}
	if days, ok := offsets[text]; ok {
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		return today.AddDate(0, 0, days), nil
	}
	return time.ParseInLocation("02.01.2006", text, now.Location())
}
