package sockparty

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"nhooyr.io/websocket"
)

// ErrNoSuchUser is returned when an invalid user is looked up.
var ErrNoSuchUser = errors.New("No such user found by that ID")

/*
UniqueIDGenerator is a function which generates a new ID for each user join.
Ensure it is sufficiently unique, e.g. random UUIDs or database usernames.
If an error is returned, the user will be disconnected.
*/
type UniqueIDGenerator func() (string, error)

/*
UserUpdateChannel is a channel sending a user's ID, used informing user joins & leaves.
*/
type UserUpdateChannel chan string

// New creates a new room for users to join.
func New(uidGenerator UniqueIDGenerator, options *Options) *Party {
	return &Party{
		UIDGenerator: uidGenerator,
		ErrorHandler: func(e error) {},

		opts:           options,
		connectedUsers: make(map[string]*user),
	}
}

// Party represents a group of users connected in a socket session.
type Party struct {
	// Human readable name of the party
	Name         string
	UIDGenerator UniqueIDGenerator

	// Called when an error occurs within the party.
	ErrorHandler func(err error)

	userJoinChannel  UserUpdateChannel
	userLeaveChannel UserUpdateChannel
	incoming         chan Incoming

	opts           *Options
	connectedUsers map[string]*user
	mut            sync.RWMutex
}

/*
ServeHTTP handles an HTTP request to join the room and upgrade to WebSocket.
It blocks until the user leaves/disconnects.
*/
func (party *Party) ServeHTTP(rw http.ResponseWriter, req *http.Request) {

	// Upgrade the HTTP request to a socket connection
	conn, err := websocket.Accept(rw, req, &websocket.AcceptOptions{
		InsecureSkipVerify: party.opts.AllowCrossOrigin,
	})
	if err != nil {
		party.ErrorHandler(fmt.Errorf("failed to upgrade websocket connection: %v", err))
		return
	}

	/* Party's incoming channel is passed to new users, so all incoming data
	is funnelled back to the consumer. */
	uid, err := party.UIDGenerator()
	if err != nil {
		party.ErrorHandler(fmt.Errorf("failed to generate unique ID: %v", err))
		conn.Close(websocket.StatusInternalError, "User creation failed")
		return
	}
	usr := newUser(
		uid,
		party.incoming,
		conn,
		party.opts,
	)

	// Add the user and begin processing
	party.addUser(usr)
	closed := make(chan error)
	go usr.listen(req.Context(), closed)
	for {
		select {
		case err := <-closed:
			// User listen closed
			if err != nil {
				go party.ErrorHandler(err)
			}
			party.removeUser(usr.ID)
			return
		}
	}
}

// UserExists returns true if the user's ID was matched in this party.
func (party *Party) UserExists(userID string) bool {
	party.mut.RLock()
	defer party.mut.RUnlock()
	_, ok := party.connectedUsers[userID]
	return ok
}

/*
GetConnectedUserIDs returns a list of all currently connected user's IDs,
this is O(n). */
func (party *Party) GetConnectedUserIDs() []string {
	party.mut.RLock()
	defer party.mut.RUnlock()

	userIDs := make([]string, len(party.connectedUsers))
	i := 0
	for id := range party.connectedUsers {
		userIDs[i] = id
		i++
	}
	return userIDs
}

// GetConnectedUserCount returns the number of currently connected users.
func (party *Party) GetConnectedUserCount() int {
	party.mut.RLock()
	defer party.mut.RUnlock()
	return len(party.connectedUsers)
}

// Broadcast writes a single outgoing message to all users currently active in the party.
func (party *Party) Broadcast(ctx context.Context, message *Outgoing) error {
	party.mut.RLock()
	defer party.mut.RUnlock()
	for _, usr := range party.connectedUsers {
		usr.write(ctx, message)
	}
	return nil
}

// Message writes a single outgoing message to a user by their ID.
func (party *Party) Message(ctx context.Context, userID string, message *Outgoing) error {
	party.mut.RLock()
	defer party.mut.RUnlock()
	if usr, ok := party.connectedUsers[userID]; ok {
		return usr.write(ctx, message)
	}
	return ErrNoSuchUser
}

/*
End attempts to remove all users from the party, closing the underlying socket connections
with a message.
*/
func (party *Party) End(message string) {
	party.mut.Lock()
	defer party.mut.Unlock()
	for _, user := range party.connectedUsers {
		user.close(message)
		delete(party.connectedUsers, user.ID)
	}
}

/*
RegisterIncoming registers the channel to be used for all incoming user messages,
replacing the previous if any; this is a fan-in style API, if there is no receiver,
the party will block.
*/
func (party *Party) RegisterIncoming(ch chan Incoming) {
	party.incoming = ch
}

/*
RegisterOnUserJoined registers the channel to be used for sending user information
when a user has joined, replacing the previous if any; if registered, the consumer
must listen on it to avoid blocking the party. When this is sent into, the user has
already joined the party, and is valid to message.
*/
func (party *Party) RegisterOnUserJoined(ch UserUpdateChannel) {
	party.userJoinChannel = ch
}

/*
RegisterOnUserLeft registers the channel to be used for sending user information
when a user has left the party, replacing the previous if any; if registered, the consumer
must listen on it to avoid blocking the party. When this is sent into, the user has already
left the party, and is no longer valid to message.
*/
func (party *Party) RegisterOnUserLeft(ch UserUpdateChannel) {
	party.userLeaveChannel = ch
}

/* Write locks should be released before callbacks,
to prevent deadlocking if callback attempts to read or write. */

// Remove the user from the party's list, and run callbacks.
func (party *Party) removeUser(id string) error {
	party.mut.Lock()
	if user, ok := party.connectedUsers[id]; ok {
		delete(party.connectedUsers, user.ID)
		party.mut.Unlock()
		if party.userLeaveChannel != nil {
			party.userLeaveChannel <- id
		}
		return nil
	}
	party.mut.Unlock()
	return ErrNoSuchUser
}

// Add a user to the party's list, and run callbacks.
func (party *Party) addUser(usr *user) {
	party.mut.Lock()
	party.connectedUsers[usr.ID] = usr
	party.mut.Unlock()

	if party.userJoinChannel != nil {
		party.userJoinChannel <- usr.ID
	}
}
