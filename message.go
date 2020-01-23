package sockparty

// MessageEvent is a string representing a message's event type.
type MessageEvent string

// MessageHandler is a function type for callbacks receiving party messages.
type MessageHandler func(party *Party, message IncomingMessage)

// Event types
const (
	ChatMessageEvent = "chat_message"
)

// IncomingMessage represents a socket message from a user, destined to the server.
type IncomingMessage struct {
	Event   MessageEvent `json:"event"`
	UserID  string       `json:"-"`
	Payload []byte       `json:"payload"`
}

/*OutgoingMessage represents a message destined from the server to users.
Broadcast can be set to true to indicate the message is for all users,
otherwise the message can be sent to a specific user ID. */
type OutgoingMessage struct {
	Broadcast bool
	UserID    string       `json:"-"`
	Event     MessageEvent `json:"event"`
	Payload   interface{}  `json:"payload"`
}
