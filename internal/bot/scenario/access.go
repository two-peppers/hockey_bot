package scenario

import (
	"errors"
	"hockey_bot/internal/domain"
	"hockey_bot/internal/storage"
	"slices"
)

var ErrForbidden = errors.New("недостаточно прав для управления командой")
var ErrNotFound = errors.New("запись не найдена; начните действие заново")

func requireTeamAdmin(st storage.Storage, teamID string, userID int64) (*domain.Team, error) {
	team, err := st.GetTeamByID(teamID)
	if err != nil {
		return nil, err
	}
	if team == nil {
		return nil, ErrNotFound
	}
	if !slices.Contains(team.Admins, userID) {
		return nil, ErrForbidden
	}
	return team, nil
}

func getTraining(st storage.Storage, id string) (*domain.Training, error) {
	training, err := st.GetTrainingByID(id)
	if err != nil {
		return nil, err
	}
	if training == nil {
		return nil, ErrNotFound
	}
	return training, nil
}

func getOwnedPlayer(st storage.Storage, id, teamID string, userID int64) (*domain.Player, error) {
	player, err := st.GetPlayerByID(id)
	if err != nil {
		return nil, err
	}
	if player == nil {
		return nil, ErrNotFound
	}
	if player.TeamID != teamID || player.TelegramID != userID {
		return nil, ErrForbidden
	}
	return player, nil
}
