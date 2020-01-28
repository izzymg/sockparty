package sockparty

import (
	"context"
	"fmt"
	"net/http"

	"nhooyr.io/websocket"
)

// NewParty creates a new room for users to join.
func NewParty(name string, incoming chan IncomingMessage, outgoing chan OutgoingMessage, options *Options) *Party {
	return &Party{
		Name:        name,
		Options:     options,
		SendMessage: outgoing,
		Incoming:    incoming,

		UserAddedHandler:          func(p *Party, u *User) {},
		UserRemovedHandler:        func(p *Party, u *User) {},
		UserInvalidMessageHandler: func(p *Party, u *User, m IncomingMessage) {},
		ErrorHandler:              func(e error) {},

		connectedUsers: make(map[string]*User),
		addUser:        make(chan *User),
		removeUser:     make(chan *User),
	}
}

// Party represents a group of users connected in a socket session.
type Party struct {
	// Human readable name of the party
	Name string
	// Configuration options for the party
	Options *Options
	// Send an outgoing message to users
	SendMessage chan OutgoingMessage
	// Receive messages
	Incoming chan IncomingMessage

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

	// Generate a new user
	user, err := newUser(
		party.Incoming,
		conn,
		party.Options,
	)

	if err != nil {
		go party.ErrorHandler(fmt.Errorf("Failed to create new user: %v", err))
		conn.Close(websocket.StatusInternalError, "User creation failure.")
		return
	}

	// Add the user and begin processing
	party.addUser <- user
	closed := make(chan error)
	go user.Listen(req.Context(), closed)
	for {
		select {
		case err := <-closed:
			// User listen closed
			if err != nil {
				go party.ErrorHandler(err)
			}
			party.removeUser <- user
			return
		}
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

/*
Listen listens on the party's data channels and process any requests related to users.
When the context is canceled, the party will attempt to cleanup all user
connections, and this routine will exit.
*/
func (party *Party) Listen(ctx context.Context) {
	for {
		select {

		case <-ctx.Done():
			party.removeUsers()
			return
		case user := <-party.addUser:
			// Add user to map
			party.connectedUsers[user.ID] = user
			go party.UserAddedHandler(party, user)

		case user := <-party.removeUser:
			// Remove user from map
			delete(party.connectedUsers, user.ID)
			go party.UserRemovedHandler(party, user)
		}
	}
}

// Close all user connections and remove them from the party.
func (party *Party) removeUsers() {
	for _, user := range party.connectedUsers {
		user.Close("Party ending.")
		delete(party.connectedUsers, user.ID)
	}
}

// Push to all users
func (party *Party) broadcast(ctx context.Context, message *OutgoingMessage) {
	for _, user := range party.connectedUsers {
		message.UserID = user.ID
		party.messageUser(ctx, message)
	}
}

// Push to one user
func (party *Party) messageUser(ctx context.Context, message *OutgoingMessage) {
	if user, ok := party.connectedUsers[message.UserID]; ok {
		user.SendOutgoing(ctx, message)
	}
}
