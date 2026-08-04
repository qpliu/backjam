package main

import (
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Server     string
	ClientName string
	Dir        string
	MP3Dir     string
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

	files, err := NewFiles(config.Dir, config.MP3Dir)
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
	var currentParams StreamerParams
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
		case ".bpm":
			if currentFile == nil || currentFile.Tempo <= 0 {
			} else if i, err := strconv.ParseInt(arg, 10, 0); err == nil {
				currentParams.Speed = int(100 * float64(i) / float64(currentFile.Tempo))
				streamer.SendChat(strconv.Itoa(currentParams.Speed))
			}
		case ".ci", ".countin":
			if arg == "on" {
				currentParams.CountIn = true
			} else if arg == "off" {
				currentParams.CountIn = false
			}
			if currentParams.CountIn && currentFile != nil && currentFile.CountIn > 0 {
				streamer.SendChat("Count-in is on")
			} else {
				streamer.SendChat("Count-in is off")
			}
		case ".f", ".file":
			if arg == "" {
				if currentFile != nil {
					streamer.SendChat(currentFile.GetDescription())
				}
			} else if file, err := files.LoadFile(arg); err != nil {
				streamer.SendChat(err.Error())
			} else {
				currentFile = file
				currentParams = StreamerParams{
					Volume:      file.Volume,
					PitchShift:  file.PitchShift,
					StemVolumes: file.StemVolumes(),
					CountIn:     file.CountIn > 0,
				}
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
				bar, err := strconv.Atoi(currentTag)
				isBar := err == nil
				if bar == 0 {
					isBar = false
				}
				for _, cm := range currentFile.ChatMessages {
					if isBar && bar < cm.Bar {
						streamer.SendChat("*" + currentTag)
						isBar = false
					}
					if cm.Tag != "" {
						if currentTag == cm.Tag || (isBar && cm.Bar == bar) {
							streamer.SendChat("*" + cm.Tag)
							isBar = false
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
				} else if err := streamer.Stream(currentFile, currentParams); err != nil {
					currentFile = nil
					currentParams = StreamerParams{}
					streamer.SendChat(err.Error())
				}
			} else if arg[0] == '@' && currentFile != nil {
				currentParams.Tag = arg[1:]
				if err := streamer.Stream(currentFile, currentParams); err != nil {
					currentFile = nil
					currentParams = StreamerParams{}
					streamer.SendChat(err.Error())
				}
			} else if file, err := files.LoadFile(arg); err != nil {
				currentFile = nil
				currentParams = StreamerParams{}
				streamer.SendChat(err.Error())
			} else {
				currentParams = StreamerParams{
					Volume:      file.Volume,
					PitchShift:  file.PitchShift,
					StemVolumes: file.StemVolumes(),
					CountIn:     file.CountIn > 0,
				}
				if err := streamer.Stream(file, currentParams); err != nil {
					currentFile = nil
					currentParams = StreamerParams{}
					streamer.SendChat(err.Error())
				} else {
					currentFile = file
					streamer.SendChat(file.GetDescription())

				}
			}
		case ".ps", ".pitch", ".pitchshift":
			if i, err := strconv.ParseInt(arg, 10, 0); err == nil {
				currentParams.PitchShift = int(i)
			}
			streamer.SendChat(strconv.Itoa(currentParams.PitchShift))
		case ".s", ".stop":
			streamer.StopStream()
		case ".sp", ".speed":
			if i, err := strconv.ParseInt(arg, 10, 0); err == nil {
				currentParams.Speed = int(i)
			}
			streamer.SendChat(strconv.Itoa(currentParams.Speed))
		case ".st", ".stem", ".stems":
			if currentFile != nil {
				if arg != "" {
					tag, vol, _ := strings.Cut(arg, " ")
					if v, err := strconv.ParseInt(vol, 10, 0); err == nil {
						for i, stem := range currentFile.Stems {
							if stem.Tag == tag {
								currentParams.StemVolumes[i] = int(v)
								break
							}
						}
					}
				}
				for i, stem := range currentFile.Stems {
					streamer.SendChat(stem.Tag + " " + strconv.Itoa(currentParams.StemVolumes[i]))
				}
			}
		case ".v", ".volume":
			if i, err := strconv.ParseInt(arg, 10, 0); err == nil {
				currentParams.Volume = int(i)
			}
			streamer.SendChat(strconv.Itoa(currentParams.Volume))
		case ".x", ".disconnect":
			wg.Done()
		case ".?", ".h", ".help":
			for _, s := range []string{
				"Bot control commands:",
				".bpm [bpm]   set playback speed (beats per minute) ",
				".ci [on|off] turn count-in on/off",
				".f           show selected file",
				".f [file]    select file",
				".l           list files",
				".l [prefix]  list files starting with prefix",
				".m           show section markers of selected file",
				".m [marker]  select section at which to start playing",
				".p           play selected file",
				".p [file]    play file",
				".p @[marker] play selected file starting at specified section",
				".ps [shift]  set pitch shift (cents)",
				".s           stop playing",
				".sp [speed]  set playback speed (percentage)",
				".st          list stems",
				".st [stem] [volume] set stem volume",
				".x           disconnect",
				".v [volume]  set volume (percentage)",
				".?           display this help",
			} {
				streamer.SendChat(s)
			}
		}
	}
}
