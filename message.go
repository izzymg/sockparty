package sockparty

// Message is a struct needing documentation and a new home.
type Message struct {
	Event       string      `json:"event"`
	Destination string      `json:"destination"`
	Payload     interface{} `json:"payload"`
}

// MessageHandler is a function type for callbacks receiving party messages.
type MessageHandler func(party *Party, message *Message)

// Event types
const (
	ChatMessage = "chat_message"
)
