package sockparty

import (
	"context"
	"fmt"
	"net/http"

	"golang.org/x/time/rate"
	"nhooyr.io/websocket"
)

// NewParty creates a new room for users to join.
func NewParty(name string, options *Options) *Party {
	return &Party{
		Name:           name,
		Options:        options,
		Stop:           make(chan bool),
		Broadcast:      make(chan Message),
		MessageUser:    make(chan Message),
		connectedUsers: make(map[string]*User),
		addUser:        make(chan *User),
		deleteUser:     make(chan *User),
		handlers: map[string]MessageHandler{
			ChatMessage: options.ChatMessageHandler,
		},
	}
}

// Options configures a party's settings.
type Options struct {
	// The origin header that must be present for users to connect.
	AllowedOrigin string
	// Limiter used against incoming client messages.
	RateLimiter *rate.Limiter

	// Handler for chat messages
	ChatMessageHandler MessageHandler
}

// Party represents a group of users connected in a socket session.
type Party struct {
	// Human readable name of the party
	Name string
	// Configuration options for the party
	Options *Options
	// Close or send to this channel to stop the party from running
	Stop chan bool
	// Write a message to the user at ID of the message's destination
	MessageUser chan Message
	// Broadcast message to all users
	Broadcast chan Message

	// Connections currently active in this party
	connectedUsers map[string]*User

	addUser    chan *User
	deleteUser chan *User
	handlers   map[string]MessageHandler
}

/*ServeHTTP handles an HTTP request to join the room and upgrade to WebSocket. As this adds users to the party,
it will block indefinitely if the party is not currently listening. */
func (party *Party) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	fmt.Println("Party: got request")

	// Upgrade the HTTP request to a socket connection
	conn, err := websocket.Accept(rw, req, &websocket.AcceptOptions{})
	if err != nil {
		fmt.Printf("Party: Failed to upgrade websocket connection: %v", err)
		return
	}

	// Generate a new user structure
	user, err := newUser(
		fmt.Sprintf("User"),
		conn,
		party.Options,
	)

	fmt.Printf("User %s joined\n", user.ID)

	if err != nil {
		fmt.Printf("Party: Failed to create new user: %v", err)
		conn.Close(websocket.StatusInternalError, "User creation failure.")
		return
	}

	// Add the user and begin processing
	party.addUser <- user
	err = party.processUser(req.Context(), user)
	if err != nil {
		fmt.Println(err)
	}
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

		case <-party.Stop:
			fmt.Println("Party stopped")
			return

		case user := <-party.addUser:
			// Add user to map
			party.connectedUsers[user.ID] = user
			fmt.Printf("Party: added user %s\n", user.ID)

		case user := <-party.deleteUser:
			// Remove user from map
			delete(party.connectedUsers, user.ID)
			fmt.Printf("Party: removed user %s\n", user.ID)

		case message := <-party.Broadcast:
			// Broadcast message to all users in map
			for _, user := range party.connectedUsers {
				select {
				// Don't block if the user isn't available.
				// TODO: Timeout here.
				case user.toUser <- message:
					fmt.Println("Party: broadcast successful")
				default:
					fmt.Printf("Party: broadcast to user %s skipped, no receiver\n", user.ID)
				}
			}

		case message := <-party.MessageUser:
			// Message single user using the destination field
			user, ok := party.connectedUsers[message.Destination]
			if !ok {
				fmt.Println("Party: message to non-existent user")
			}
			// Don't block if the user isn't available.
			// TODO: Timeout here.
			select {
			case user.toUser <- message:
				fmt.Printf("Party: messaged user %s\n", user.ID)
			default:
				fmt.Printf("Party: user %s not receiving message\n", user.ID)
			}
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
			party.deleteUser <- user
			return fmt.Errorf("Party: process user closed: %w", err)
		case message := <-user.fromUser:
			handler := party.handlers[message.Event]
			fmt.Printf("Party: message %v, handler: %v\n", message, handler)
		}
	}
}
