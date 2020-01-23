package sockparty

// MessageEvent is a string representing a message's event type.
type MessageEvent string

// MessageHandler is a function type for callbacks receiving party messages.
type MessageHandler func(party *Party, message IncomingMessage)

// Event types
const (
	ChatMessageEvent = "chat_message"
)

// IncomingMessage contains information about a message, including its type, the destination and the source.
type IncomingMessage struct {
	Event      MessageEvent `json:"event"`
	SourceUser string
	Payload    []byte `json:"payload"`
}

// OutgoingMessage contains information about a message, including its type, the destination and the source.
type OutgoingMessage struct {
	Event   MessageEvent `json:"event"`
	Payload interface{}  `json:"payload"`
}
