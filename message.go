package sockparty

// Message is a struct needing documentation and a new home.
type Message struct {
	Event       string      `json:"event"`
	Destination string      `json:"destination"`
	Payload     interface{} `json:"payload"`
}

type MessageHandler func(party *Party, message *Message)

const (
	ChatMessage = "chat_message"
)
