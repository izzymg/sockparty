package sockparty

import (
	"context"
	"fmt"
	"net/http"

	"nhooyr.io/websocket"
)

// NewParty creates a new room for users to join.
func NewParty(name string, options *Options) *Party {
	return &Party{
		Name:          name,
		Options:       options,
		StopListening: make(chan bool),
		SendMessage:   make(chan OutgoingMessage),

		UserAddedHandler:          func(p *Party, u *User) {},
		UserRemovedHandler:        func(p *Party, u *User) {},
		UserInvalidMessageHandler: func(p *Party, u *User, m IncomingMessage) {},
		ErrorHandler:              func(e error) {},

		messageHandlers: make(map[MessageEvent]MessageHandler),
		connectedUsers:  make(map[string]*User),
		addUser:         make(chan *User),
		removeUser:      make(chan *User),
	}
}

// Party represents a group of users connected in a socket session.
type Party struct {
	// Human readable name of the party
	Name string
	// Configuration options for the party
	Options *Options
	// Close or send to this channel to stop the party from running
	StopListening chan bool
	// Send an outgoing message to users
	SendMessage chan OutgoingMessage

	ErrorHandler              func(err error)
	UserAddedHandler          func(party *Party, user *User)
	UserRemovedHandler        func(party *Party, user *User)
	UserInvalidMessageHandler func(party *Party, user *User, message IncomingMessage)

	// Connections currently active in this party
	connectedUsers map[string]*User

	messageHandlers map[MessageEvent]MessageHandler

	addUser    chan *User
	removeUser chan *User
}

/*ServeHTTP handles an HTTP request to join the room and upgrade to WebSocket. As this adds users to the party,
it will block indefinitely if the party is not currently listening. */
func (party *Party) ServeHTTP(rw http.ResponseWriter, req *http.Request) {

	// Upgrade the HTTP request to a socket connection
	conn, err := websocket.Accept(rw, req, &websocket.AcceptOptions{
		InsecureSkipVerify: party.Options.AllowCrossOrigin,
	})
	if err != nil {
		party.ErrorHandler(fmt.Errorf("Failed to upgrade websocket connection: %v", err))
		return
	}

	// Generate a new user structure
	user, err := newUser(
		"User",
		conn,
		party.Options,
	)

	if err != nil {
		party.ErrorHandler(fmt.Errorf("Failed to create new user: %v", err))
		conn.Close(websocket.StatusInternalError, "User creation failure.")
		return
	}

	// Add the user and begin processing
	party.addUser <- user

	// Blocks
	err = party.processUser(req.Context(), user)
	// User has left, processing has ended
	if err != nil {
		party.ErrorHandler(err)
	}
}

// SetMessageEvent sets the handler for messages with some event type to a handler function.
func (party *Party) SetMessageEvent(event MessageEvent, handler MessageHandler) {
	party.messageHandlers[event] = handler
}

// GetConnectedUserCount returns the number of currently connected users.
func (party *Party) GetConnectedUserCount() int {
	return len(party.connectedUsers)
}

/*Listen listens on the party's data channels and process any requests related to users.
This blocks, and can be stopped by closing the party's Stop channel field. */
func (party *Party) Listen() {
	for {
		select {

		case <-party.StopListening:
			return

		case message := <-party.SendMessage:
			if message.Broadcast {
				party.broadcast(message)
			} else {
				party.messageUser(message)
			}

		case user := <-party.addUser:
			// Add user to map
			party.connectedUsers[user.ID] = user
			party.UserAddedHandler(party, user)

		case user := <-party.removeUser:
			// Remove user from map
			delete(party.connectedUsers, user.ID)
			party.UserRemovedHandler(party, user)
		}
	}
}

// Push to all users
func (party *Party) broadcast(message OutgoingMessage) {
	for _, user := range party.connectedUsers {
		message.UserID = user.ID
		party.messageUser(message)
	}
}

// Push to one user
func (party *Party) messageUser(message OutgoingMessage) error {
	if user, ok := party.connectedUsers[message.UserID]; ok {
		// Don't block if the user isn't available.
		// TODO: Timeout here.
		select {
		case user.toUser <- message:
			break
		default:
			return fmt.Errorf("User blocked %s", user.ID)
		}
	} else {
		return fmt.Errorf("No such user %s", message.UserID)
	}
	return nil
}

/* Process user begins processing the user's requests.
If the HTTP request is canceled for any reason, the context will go with it,
cascading and cleaning up. This function blocks, waiting for any messages from the user
or the user to close before deleting it and returning. */
func (party *Party) processUser(ctx context.Context, user *User) error {

	// Begin processing incoming and outgoing data.
	go user.listenIncoming(ctx)
	go user.listenOutgoing(ctx)

	for {
		select {
		case err := <-user.closed:
			// Delete and cleanup a closed user
			party.removeUser <- user
			return fmt.Errorf("Party: process user closed: %w", err)
		case message := <-user.fromUser:
			// Pass incoming messages to assigned handler
			if handler, ok := party.messageHandlers[message.Event]; ok {
				handler(party, message)
				continue
			}
			party.UserInvalidMessageHandler(party, user, message)
		}
	}
}
