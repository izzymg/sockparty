package sockmessages

import "encoding/json"

// Event is a string representing a message's event type.
type Event string

// Incoming represents a socket message from a user, destined to the server.
type Incoming struct {
	Event   Event           `json:"event"`
	UserID  string          `json:"-"`
	Payload json.RawMessage `json:"payload"`
}

/*
Outgoing represents a message destined from the server to users.
Broadcast can be set to true to indicate the message is for all users,
otherwise the message can be sent to a specific user ID.
*/
type Outgoing struct {
	Broadcast bool        `json:"-"`
	UserID    string      `json:"-"`
	Event     Event       `json:"event"`
	Payload   interface{} `json:"payload"`
}
