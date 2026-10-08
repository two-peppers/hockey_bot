package user_state

import "sync"

type MessageInfo struct {
	ChatID    int64
	MessageID int
}

var globalMessageBuffer = make(map[string][]MessageInfo)
var bufferMu sync.Mutex

func AddToMessageBuffer(chatID int64, userID int64, MessageID int) {
	bufferMu.Lock()
	defer bufferMu.Unlock()
	key := buildKey(userID, chatID)

	newElem := MessageInfo{
		ChatID:    chatID,
		MessageID: MessageID,
	}

	globalMessageBuffer[key] = append(globalMessageBuffer[key], newElem)
}

func DrainMessageBuffer(userID, chatID int64) []MessageInfo {
	bufferMu.Lock()
	defer bufferMu.Unlock()
	key := buildKey(userID, chatID)
	messages := globalMessageBuffer[key]
	delete(globalMessageBuffer, key)
	return messages
}
