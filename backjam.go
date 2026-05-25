package main

import (
	"os"
	"strings"
	"sync"
)

func main() {
	const server = "localhost:22124"
	const clientName = "backjam-bot"

	streamer, err := NewStreamer(server, clientName)
	if err != nil {
		panic(err.Error())
	}

	var wg sync.WaitGroup
	wg.Add(1)

	iarg := 1
	streamer.SetOnChatReceived(func(text string) {
		switch {
		case strings.HasSuffix(text, "> .x"):
			wg.Done()
		case strings.HasSuffix(text, "> .s"):
			streamer.StopStream()
		case strings.HasSuffix(text, "> .p") && len(os.Args) > 1:
			if iarg > len(os.Args) {
				iarg = 1
			}
			err := streamer.Stream(os.Args[iarg], nil, 0)
			if err != nil {
				streamer.SendChat(err.Error())
			}
		}
	})

	wg.Wait()
	streamer.Close()
}
