package main

import (
	"os"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Server     string
	ClientName string
	Dir        string
	Users      []string
}

func main() {
	config := Config{
		Server:     "localhost:22124",
		ClientName: "backjam-bot",
		Dir:        "./music",
	}

	if len(os.Args) > 1 {
		if _, err := toml.DecodeFile(os.Args[1], &config); err != nil {
			panic(err.Error())
		}
	}

	files, err := NewFiles(config.Dir)
	if err != nil {
		panic(err.Error())
	}

	streamer, err := NewStreamer(config.Server, config.ClientName)
	if err != nil {
		panic(err.Error())
	}

	var wg sync.WaitGroup
	wg.Add(1)
	streamer.SetOnChatReceived(ChatCommandHandler(config, files, streamer, &wg))
	wg.Wait()
	streamer.Close()
}

func ChatCommandHandler(config Config, files *Files, streamer *Streamer, wg *sync.WaitGroup) func(string) {
	return func(text string) {
		_, text, _ = strings.Cut(text, "<b>")
		user, text, _ := strings.Cut(text, "</b></font> ")
		cmd, arg, _ := strings.Cut(text, " ")

		if user == "" || user == config.ClientName {
			return
		}
		if len(config.Users) > 0 {
			userOk := false
			for _, configUser := range config.Users {
				if configUser == user {
					userOk = true
					break
				}
			}
			if !userOk {
				return
			}
		}
		switch cmd {
		case ".x":
			wg.Done()
		case ".ls":
			files.Rescan()
			for _, text := range files.List(arg) {
				streamer.SendChat(text)
			}
		case ".p":
			if arg == "" {
				streamer.StopStream()
			} else if file, err := files.LoadFile(arg); err != nil {
				streamer.SendChat(err.Error())
			} else if err := streamer.Stream(file.GetAudioFileName(), file.GetChatMessages(), 0); err != nil {
				streamer.SendChat(err.Error())
			} else {
				streamer.SendChat(file.GetDescription())
			}
		}
	}
}
