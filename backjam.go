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
	var currentFile *File
	var currentTag string
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
		case ".f", ".file":
			if arg == "" {
				if currentFile != nil {
					streamer.SendChat(currentFile.GetDescription())
				}
			} else if file, err := files.LoadFile(arg); err != nil {
				streamer.SendChat(err.Error())
			} else {
				currentFile = file
				currentTag = ""
				streamer.SendChat(file.GetDescription())
			}
		case ".l", ".ls", ".list":
			files.Rescan()
			for _, text := range files.List(arg) {
				streamer.SendChat(text)
			}
		case ".m", ".marker":
			if currentFile == nil {
			} else if arg == "" {
				for _, cm := range currentFile.ChatMessages {
					if cm.Tag != "" {
						if currentTag == cm.Tag {
							streamer.SendChat("*" + cm.Tag)
						} else {
							streamer.SendChat(cm.Tag)
						}
					}
				}
			} else {
				currentTag = arg
			}
		case ".p", ".play":
			if arg == "" {
				if currentFile == nil {
				} else if err := streamer.Stream(currentFile.GetAudioFileName(), currentFile.GetChatMessages(), currentFile.GetOffset(currentTag)); err != nil {
					currentFile = nil
					currentTag = ""
					streamer.SendChat(err.Error())
				}
			} else if file, err := files.LoadFile(arg); err != nil {
				currentFile = nil
				currentTag = ""
				streamer.SendChat(err.Error())
			} else if err := streamer.Stream(file.GetAudioFileName(), file.GetChatMessages(), 0); err != nil {
				currentFile = nil
				currentTag = ""
				streamer.SendChat(err.Error())
			} else {
				currentFile = nil
				currentTag = ""
				streamer.SendChat(file.GetDescription())
			}
		case ".s", ".stop":
			streamer.StopStream()
		case ".x", ".disconnect":
			wg.Done()
		case ".?", ".h", ".help":
			for _, s := range []string{
				"Bot control commands:",
				".f          show selected file",
				".f [file]   select file",
				".l          list files",
				".l [prefix] list files starting with prefix",
				".m          show section markers of selected file",
				".m [marker] select section at which to start playing",
				".p          play selected file",
				".p [file]   play file",
				".s          stop playing",
				".x          disconnect",
				".?          display this help",
			} {
				streamer.SendChat(s)
			}
		}
	}
}
