package user_state

import (
	"fmt"
	"sync"
)

var globalUserStates = make(map[string]string)
var stateMu sync.RWMutex

func GetUserState(userID int64, chatID int64) string {
	stateMu.RLock()
	state := globalUserStates[buildKey(userID, chatID)]
	stateMu.RUnlock()

	return state
}

func SetUserState(userID int64, chatID int64, state string) {
	stateMu.Lock()
	globalUserStates[buildKey(userID, chatID)] = state
	stateMu.Unlock()
}

func ClearUserState(userID int64, chatID int64) {
	stateMu.Lock()
	delete(globalUserStates, buildKey(userID, chatID))
	stateMu.Unlock()
}

func buildKey(userID int64, chatID int64) string {
	return fmt.Sprintf("%d_%d", userID, chatID)
}
