package sockparty

import (
	"encoding/json"

	"github.com/google/uuid"
)

// Event is a string representing a message's event type.
type Event string

/*
Incoming represents a socket message from a user, destined to the server.
The UserID is the user who sent the message to the server.
The payload is raw JSON containing arbitrary information from the client.
*/
type Incoming struct {
	Event   Event           `json:"event"`
	UserID  uuid.UUID       `json:"-"`
	Payload json.RawMessage `json:"payload"`
}

/*
Outgoing represents a message destined from the server to users.
It contains an event to inform the client of the type of message,
and the payload containing the actual message data of any type.
*/
type Outgoing struct {
	Event   Event       `json:"event"`
	Payload interface{} `json:"payload"`
}
