package main

import (
	"fmt"
	"os"

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

	client.SetOnRawAudioSupported(func() {
		fmt.Printf("raw audio supported\n")
	})

	StreamMP3(input, client)
}
