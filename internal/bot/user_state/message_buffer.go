package user_state

type MessageInfo struct {
	ChatID    int64
	MessageID int
}

var globalMessageBuffer = make(map[int64][]MessageInfo)

func AddToMessageBuffer(chatID int64, userID int64, MessageID int) {
	if _, ok := globalMessageBuffer[userID]; !ok {
		globalMessageBuffer[userID] = make([]MessageInfo, 0)
	}

	newElem := MessageInfo{
		ChatID:    chatID,
		MessageID: MessageID,
	}

	globalMessageBuffer[userID] = append(globalMessageBuffer[userID], newElem)
}

func GetUserMessageBuffer(userID int64) []MessageInfo {
	if _, ok := globalMessageBuffer[userID]; !ok {
		return make([]MessageInfo, 0)
	}

	return globalMessageBuffer[userID]
}
