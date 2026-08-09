package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

type Files struct {
	dir   string
	items map[string]fileType
}

type File struct {
	AudioFileName   string
	Description     string
	Tempo           int
	CountIn         int
	CountInOffsetMs int
	Bar             struct {
		Beats int
	}
	Bars          map[string]float64
	StartOffsetMs int
	ChatMessages  []struct {
		Tag  string
		Text string
		Bar  int
	}
	Volume     int
	PitchShift int
	Stems      map[string]StemFile
}

type StemFile struct {
	AudioFileName string
	Volume        int
}

type fileType int

const (
	fileTypeDir fileType = iota
	fileTypeTOML
	fileTypeMP3
)

func NewFiles(dir string) (*Files, error) {
	fs := &Files{dir: dir, items: make(map[string]fileType)}
	fs.Rescan()
	return fs, nil
}

func (fs *Files) Rescan() {
	items := make(map[string]fileType)
	fs.scan("", items)
	fs.items = items
}

func (fs *Files) scan(dir string, items map[string]fileType) {
	entries, err := os.ReadDir(filepath.Join(fs.dir, dir))
	if err != nil {
		panic(err.Error())
	}
	for _, entry := range entries {
		name := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			if _, ok := items[name]; !ok {
				items[name] = fileTypeDir
			}
			fs.scan(name, items)
		} else if strings.HasSuffix(name, ".toml") {
			name = name[:len(name)-5]
			items[name] = fileTypeTOML
		} else if strings.HasSuffix(name, ".mp3") {
			name = name[:len(name)-4]
			if items[name] != fileTypeTOML {
				items[name] = fileTypeMP3
			}
		}
	}
}

func (fs *Files) List(arg string) []string {
	results := fs.matching(arg, true)
	sort.Strings(results)
	return results
}

func (fs *Files) matching(arg string, includeDirs bool) []string {
	if itemType, ok := fs.items[arg]; ok && !includeDirs && itemType != fileTypeDir {
		return []string{arg}
	}
	results := []string{}
	for k, itemType := range fs.items {
		if itemType == fileTypeDir && !includeDirs {
			continue
		}
		if ok, _ := filepath.Match(arg, k); ok {
		} else if !strings.HasPrefix(k, arg) {
			continue
		} else if strings.ContainsRune(k[len(arg):], '/') {
			continue
		}
		if itemType == fileTypeDir {
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
	switch fs.items[matching[0]] {
	case fileTypeTOML:
		f := &File{}
		name := filepath.Join(fs.dir, matching[0])
		if _, err := toml.DecodeFile(fmt.Sprintf("%s.toml", name), f); err != nil {
			return nil, err
		}
		if f.AudioFileName != "" {
			f.AudioFileName = filepath.Join(fs.dir, f.AudioFileName)
		} else {
			f.AudioFileName = fmt.Sprintf("%s.mp3", name)
		}
		if f.Stems == nil {
			f.Stems = map[string]StemFile{}
		}
		stemFiles, _ := os.ReadDir(name)
		for _, entry := range stemFiles {
			entryName := entry.Name()
			if !strings.HasSuffix(entryName, ".mp3") {
				continue
			}
			tag := entryName[:len(entryName)-4]
			if stem, ok := f.Stems[tag]; ok {
				if stem.AudioFileName == "" {
					stem.AudioFileName = filepath.Join(name, entryName)
					f.Stems[tag] = stem
				}
			} else {
				f.Stems[tag] = StemFile{
					AudioFileName: filepath.Join(name, entryName),
				}
			}
		}
		return f, nil
	case fileTypeMP3:
		return &File{
			AudioFileName: fmt.Sprintf("%s.mp3", filepath.Join(fs.dir, matching[0])),
		}, nil
	}
	return nil, fmt.Errorf("file not found")
}

func (f *File) GetDescription() string {
	return f.Description
}

func (f *File) GetChatMessages() []ChatMessage {
	if f.Tempo <= 0 {
		return nil
	}
	chatMessages := make([]ChatMessage, len(f.ChatMessages))
	for i, cm := range f.ChatMessages {
		chatMessages[i].message = cm.Text
		if cm.Bar > 0 {
			chatMessages[i].dt = f.GetBarOffset(cm.Bar)
		}
	}
	return chatMessages
}

func (f *File) GetOffset(tag string) time.Duration {
	for _, cm := range f.ChatMessages {
		if tag == cm.Tag && cm.Bar > 0 {
			return f.GetBarOffset(cm.Bar)
		}
	}
	if bar, err := strconv.Atoi(tag); err == nil {
		return f.GetBarOffset(bar)
	}
	return 0
}

func (f *File) GetBarOffset(bar int) time.Duration {
	barLen := float64(4)
	if f.Bar.Beats > 0 {
		barLen = float64(f.Bar.Beats)
	}
	beat := float64(0)
	for i := 1; i < bar; i++ {
		if newBarLen, ok := f.Bars[strconv.Itoa(i)]; ok {
			barLen = newBarLen
		}
		beat += barLen
	}
	return time.Duration(f.StartOffsetMs)*time.Millisecond + time.Duration(beat)*time.Minute/time.Duration(f.Tempo)
}

func (f *File) StemVolumes() map[string]int {
	v := map[string]int{}
	for tag, stem := range f.Stems {
		if stem.Volume == 0 {
			v[tag] = 100
		} else if stem.Volume < 0 {
			v[tag] = 0
		} else {
			v[tag] = stem.Volume
		}
	}
	return v
}
