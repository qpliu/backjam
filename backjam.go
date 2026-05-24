package main

import (
	"os"
	"time"

	"backjam/jamulus"
)

func main() {
	input := os.Stdin

	server := "localhost:22124"
	if len(os.Args) > 1 {
		server = os.Args[1]
	}
	client, err := jamulus.NewClient(server)
	if err != nil {
		panic(err.Error())
	}
	defer client.Close()

	client.SetOnClientIDReceived(func(clientID int) {
		client.UpdateChannelName("backjam-bot")
	})

	StreamMP3(input, []ChatMessage{
		ChatMessage{5 * time.Second, "5 second"},
		ChatMessage{10 * time.Second, "10 second"},
		ChatMessage{15 * time.Second, "15 second"},
		ChatMessage{30 * time.Second, "30 second"},
		ChatMessage{45 * time.Second, "45 second"},
		ChatMessage{60 * time.Second, "60 second"},
	}, client)
}
