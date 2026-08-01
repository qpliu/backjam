package stream

import (
	"io"
	"os"
	"time"

	"github.com/hajimehoshi/go-mp3"
)

type mp3Stream struct {
	file    *os.File
	decoder *mp3.Decoder
	buffer  []byte
	index   int
	count   int
}

func MP3Stream(filename string, offset time.Duration) (Stream, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	decoder, err := mp3.NewDecoder(file)
	if err != nil {
		file.Close()
		return nil, err
	}
	if _, err := decoder.Seek(4*(int64(decoder.SampleRate())*int64(offset)/int64(time.Second)), io.SeekStart); err != nil {
		file.Close()
		return nil, err
	}
	return &mp3Stream{
		file:    file,
		decoder: decoder,
		buffer:  make([]byte, 8192),
	}, nil
}

func (s *mp3Stream) Close() {
	if s.file != nil {
		s.file.Close()
		s.file = nil
		s.count = 0
	}
}

func (s *mp3Stream) Done() bool {
	return s.index >= s.count && s.file == nil
}

func (s *mp3Stream) Read(buffer [][Channels]float64) (int, error) {
	for i := range buffer {
		for ch := range Channels {
			if ch >= 2 {
				buffer[i][ch] = 0
				continue
			}
			if s.index >= s.count {
				if s.file == nil {
					return i, nil
				}
				s.index = 0
				count, err := s.decoder.Read(s.buffer)
				if err != nil && err != io.EOF {
					return 0, err
				}
				s.count = count
				if err == io.EOF {
					s.file.Close()
					s.file = nil
				}
				if count <= 0 {
					return i, nil
				}
			}
			buffer[i][ch] = float64(int16(s.buffer[s.index]) | (int16(s.buffer[s.index+1]) << 8))
			s.index += 2
		}
	}
	return len(buffer), nil
}

func (s *mp3Stream) SampleRate() int {
	return s.decoder.SampleRate()
}
