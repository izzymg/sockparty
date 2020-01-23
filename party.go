package sockparty

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/time/rate"
	"nhooyr.io/websocket"
)

// NewParty creates a new room for users to join.
func NewParty(name string, options *Options) *Party {
	// TODO: cleaner options
	if options.PingFrequency == 0 {
		options.PingFrequency = 10 * time.Second
	}
	if options.PingTimeout == 0 {
		options.PingTimeout = 10 * time.Second
	}
	return &Party{
		Name:          name,
		Options:       options,
		StopListening: make(chan bool),
		SendMessage:   make(chan OutgoingMessage),

		UserAddedHandler:          func(id string, n string) {},
		UserRemovedHandler:        func(id string, n string) {},
		UserInvalidMessageHandler: func(i IncomingMessage) {},
		ErrorHandler:              func(e error) {},

		messageHandlers: make(map[MessageEvent]MessageHandler),
		connectedUsers:  make(map[string]*User),
		addUser:         make(chan *User),
		removeUser:      make(chan *User),
	}
}

// Options configures a party's settings.
type Options struct {
	// The origin header that must be present for users to connect.
	AllowedOrigin string
	// Limiter used against incoming client messages.
	RateLimiter *rate.Limiter

	// Determines how frequently users are pinged.
	PingFrequency time.Duration
	// Determines how long to wait on a ping before assuming the connection is dead.
	PingTimeout time.Duration
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
	UserAddedHandler          func(ID string, name string)
	UserRemovedHandler        func(ID string, name string)
	UserInvalidMessageHandler func(message IncomingMessage)

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
	conn, err := websocket.Accept(rw, req, &websocket.AcceptOptions{})
	if err != nil {
		party.ErrorHandler(fmt.Errorf("Failed to upgrade websocket connection: %v", err))
		return
	}

	// Generate a new user structure
	user, err := newUser(
		fmt.Sprintf("User"),
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
				for _, user := range party.connectedUsers {
					select {
					// Don't block if the user isn't available.
					// TODO: Timeout here.
					case user.toUser <- message:
						break
					default:
						break
					}
				}
			} else {
				if user, ok := party.connectedUsers[message.UserID]; ok {
					user.toUser <- message
				}
			}

		case user := <-party.addUser:
			// Add user to map
			party.connectedUsers[user.ID] = user
			party.UserAddedHandler(user.ID, user.Name)

		case user := <-party.removeUser:
			// Remove user from map
			delete(party.connectedUsers, user.ID)
			party.UserRemovedHandler(user.ID, user.Name)
		}
	}
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
			party.UserInvalidMessageHandler(message)
		}
	}
}
