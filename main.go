package main

import (
	"fmt"
	"net/http"
	"sockroom/client"
	"sockroom/party"
	"time"
)

func main() {
	party := party.NewRoom("Cool room", "http://localhost:80")
	server := &http.Server{
		Handler: party,
		Addr:    "localhost:3500",
	}

	stoppu := make(chan bool)
	go func(stop <-chan bool) {
		server.ListenAndServe()
		<-stop
	}(stoppu)

	fmt.Println("Server up")

	go client.DoClient()

	ticker := time.NewTicker(time.Second * 5)
	for {
		select {
		case <-stoppu:
			return
		case <-ticker.C:
			fmt.Printf("Connected users: %d\n", party.GetConnectedUserCount())
		}
	}
}
