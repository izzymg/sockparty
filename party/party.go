package party

import (
	"fmt"
	"net/http"
	"nhooyr.io/websocket"
	"sync"
)

// NewRoom creates a new room for users to join.
func NewRoom(name string, allowedOrigin string) *Party {
	return &Party{
		Name:           name,
		connectedUsers: make(map[string]*User),
		allowedOrigin:  allowedOrigin,
	}
}

// Party represents a group of users connected in a socket session.
type Party struct {
	Name           string
	connectedUsers map[string]*User
	mut            sync.Mutex
	allowedOrigin  string
}

// ServeHTTP handles an HTTP request to join the room and upgrade to WebSocket.
func (party *Party) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	fmt.Println("Party: got request")

	// Upgrade the HTTP request to a socket connection
	conn, err := websocket.Accept(rw, req, &websocket.AcceptOptions{})
	if err != nil {
		fmt.Printf("Party: Failed to upgrade websocket connection: %v", err)
		return
	}

	// Add the user to the party
	user, err := newUser(
		fmt.Sprintf("User %d", party.GetConnectedUserCount()),
		conn,
	)
	if err != nil {
		fmt.Printf("Party: Failed to create new user: %v", err)
		conn.Close(websocket.StatusInternalError, "User creation failure.")
		return
	}
	party.addUser(user)
	party.processUser(user.ID)
}

// GetConnectedUserCount returns the number of currently connected users.
func (party *Party) GetConnectedUserCount() int {
	return len(party.connectedUsers)
}

// Adds a new user to the room. Dumb op.
func (party *Party) addUser(user *User) {
	fmt.Println("Party: adding user")
	// Add the connected user
	party.mut.Lock()
	defer party.mut.Unlock()
	party.connectedUsers[user.ID] = user
}

// Removes a user from the list of connected users. Performs no cleanup - dumb op.
func (party *Party) deleteUser(ID string) {
	fmt.Println("Party: deleting user")
	party.mut.Lock()
	defer party.mut.Unlock()
	delete(party.connectedUsers, ID)
}

// Process user begins processing the user's requests, blocking.
func (party *Party) processUser(ID string) error {

	// Grab user by ID
	user, ok := party.connectedUsers[ID]
	if !ok {
		return fmt.Errorf("Failed to find user by ID key %v", ID)
	}

	// Begin its processes
	go user.startReading()
	go user.startListening()

	// Block for processed data coming from the user
	for {
		select {
		case message, ok := <-user.from:
			if !ok {
				fmt.Println("Party: reading ended")
				party.mut.Lock()
				party.deleteUser(user.ID)
				party.mut.Unlock()
				return nil
			}
			fmt.Println("Party: got data from user")
			party.broadcast(message)
		}
	}
}

// Iterates over all connected users in this room and sends data to them.
func (party *Party) broadcast(message Message) {
	fmt.Println("Party: broadcasting")
	party.mut.Lock()
	defer party.mut.Unlock()
	for _, user := range party.connectedUsers {
		user.to <- message
	}
}
