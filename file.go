package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

type Files struct {
	dir   string
	items map[string]bool
}

type File struct {
	AudioFileName string
	Description   string
	Tempo         int
	Bar           struct {
		Beats int
	}
	StartOffsetMs int
	ChatMessages  []struct {
		Tag  string
		Text string
		Beat int
		Bar  int
	}

	dir string
}

func NewFiles(dir string) (*Files, error) {
	fs := &Files{dir: dir, items: make(map[string]bool)}
	fs.Rescan()
	return fs, nil
}

func (fs *Files) Rescan() {
	items := make(map[string]bool)
	fs.scan("", items)
	fs.items = items
}

func (fs *Files) scan(dir string, items map[string]bool) {
	entries, err := os.ReadDir(filepath.Join(fs.dir, dir))
	if err != nil {
		panic(err.Error())
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			items[filepath.Join(dir, name)] = true
			fs.scan(filepath.Join(dir, name), items)
		} else if strings.HasSuffix(name, ".toml") {
			items[filepath.Join(dir, name[:len(name)-5])] = false
		}
	}
}

func (fs *Files) List(arg string) []string {
	results := fs.matching(arg, true)
	sort.Strings(results)
	return results
}

func (fs *Files) matching(arg string, includeDirs bool) []string {
	if isDir, ok := fs.items[arg]; ok && !includeDirs && !isDir {
		return []string{arg}
	}
	results := []string{}
	for k, isDir := range fs.items {
		if isDir && !includeDirs {
			continue
		}
		if ok, _ := filepath.Match(arg, k); ok {
		} else if !strings.HasPrefix(k, arg) {
			continue
		} else if strings.ContainsRune(k[len(arg):], '/') {
			continue
		}
		if isDir {
			results = append(results, fmt.Sprintf("%s/", k))
		} else {
			results = append(results, k)
		}
	}
	return results
}

func (fs *Files) LoadFile(arg string) (*File, error) {
	matching := fs.matching(arg, false)
	if len(matching) != 1 {
		return nil, fmt.Errorf("file not found")
	}
	f := &File{dir: fs.dir}
	if _, err := toml.DecodeFile(filepath.Join(fs.dir, fmt.Sprintf("%s.toml", matching[0])), f); err != nil {
		return nil, err
	}
	return f, nil
}

func (f *File) GetAudioFileName() string {
	return filepath.Join(f.dir, f.AudioFileName)
}

func (f *File) GetDescription() string {
	return f.Description
}

func (f *File) GetChatMessages() []ChatMessage {
	if f.Tempo <= 0 {
		return nil
	}
	beatDuration := time.Minute / time.Duration(f.Tempo)
	barDuration := 4 * beatDuration
	if f.Bar.Beats > 0 {
		barDuration = time.Duration(f.Bar.Beats) * beatDuration
	}
	chatMessages := make([]ChatMessage, len(f.ChatMessages))
	for i, cm := range f.ChatMessages {
		chatMessages[i].message = cm.Text
		if cm.Bar > 0 {
			chatMessages[i].dt = time.Duration(cm.Bar-1)*barDuration + time.Duration(f.StartOffsetMs)*time.Millisecond
		} else if cm.Beat > 0 {
			chatMessages[i].dt = time.Duration(cm.Beat-1)*beatDuration + time.Duration(f.StartOffsetMs)*time.Millisecond
		}
	}
	return chatMessages
}

func (f *File) GetOffset(tag string) time.Duration {
	beatDuration := time.Minute / time.Duration(f.Tempo)
	barDuration := 4 * beatDuration
	if f.Bar.Beats > 0 {
		barDuration = time.Duration(f.Bar.Beats) * beatDuration
	}
	for _, cm := range f.ChatMessages {
		if tag == cm.Tag {
			if cm.Bar > 0 {
				return time.Duration(cm.Bar-1)*barDuration + time.Duration(f.StartOffsetMs)*time.Millisecond
			} else if cm.Beat > 0 {
				return time.Duration(cm.Beat-1)*beatDuration + time.Duration(f.StartOffsetMs)*time.Millisecond
			}
		}
	}
	return 0
}
