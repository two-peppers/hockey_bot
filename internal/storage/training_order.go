package storage

import (
	"cmp"
	"hockey_bot/internal/domain"
	"slices"

	"gorm.io/gorm"
)

// У старых связей created_at может отсутствовать из-за автоматического
// сохранения many2many. SQLite rowid сохраняет порядок вставки этих строк.
// Переносим его в явное поле, чтобы дальнейшие перестроения таблицы
// и совпадения времени не влияли на очередь.
func migrateTrainingSignupOrder(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var pending []domain.TrainingPlayer
		if err := tx.Where("signup_order = 0").Order("rowid").Find(&pending).Error; err != nil {
			return err
		}
		lastOrders := make(map[string]int64)
		for _, link := range pending {
			last, ok := lastOrders[link.TrainingID]
			if !ok {
				if err := tx.Model(&domain.TrainingPlayer{}).Where("training_id = ?", link.TrainingID).
					Select("COALESCE(MAX(signup_order), 0)").Scan(&last).Error; err != nil {
					return err
				}
			}
			last++
			if err := tx.Model(&link).Update("signup_order", last).Error; err != nil {
				return err
			}
			lastOrders[link.TrainingID] = last
		}
		return nil
	})
}

// В списках тренировок у одного игрока могут быть разные места в очереди.
// Поэтому сортируем каждую тренировку по её собственным связям.
func sortTrainingPlayers(db *gorm.DB, trainings []domain.Training) error {
	if len(trainings) == 0 {
		return nil
	}
	ids := make([]string, 0, len(trainings))
	for _, training := range trainings {
		ids = append(ids, training.ID)
	}
	var links []domain.TrainingPlayer
	if err := db.Where("training_id IN ?", ids).Find(&links).Error; err != nil {
		return err
	}
	type key struct{ trainingID, playerID string }
	orders := make(map[key]int64, len(links))
	for _, link := range links {
		orders[key{link.TrainingID, link.PlayerID}] = link.SignupOrder
	}
	for _, training := range trainings {
		slices.SortFunc(training.Players, func(a, b domain.Player) int {
			return cmp.Compare(orders[key{training.ID, a.ID}], orders[key{training.ID, b.ID}])
		})
	}
	return nil
}
