package user_state

import "sync"

var globalUserStates = make(map[int64]string)
var stateMu sync.RWMutex

func GetUserState(userID int64) string {
	stateMu.RLock()
	state := globalUserStates[userID]
	stateMu.RUnlock()

	return state
}

func SetUserState(userID int64, state string) {
	stateMu.Lock()
	globalUserStates[userID] = state
	stateMu.Unlock()
}

func ClearUserState(userID int64) {
	stateMu.Lock()
	delete(globalUserStates, userID)
	stateMu.Unlock()
}
