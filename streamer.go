package main

import (
	"io"
	"os"
	"sync"
	"time"

	"github.com/hajimehoshi/go-mp3"

	"backjam/jamulus"
)

type Streamer struct {
	client *jamulus.Client
	lock   sync.Mutex
	closed bool

	file         *os.File
	decoder      *mp3.Decoder
	t0           time.Time
	t            time.Time
	chatMessages []ChatMessage
}

type ChatMessage struct {
	dt      time.Duration
	message string
}

func NewStreamer(server string, clientName string) (*Streamer, error) {
	client, err := jamulus.NewClient(server)
	if err != nil {
		return nil, err
	}
	client.SetOnClientIDReceived(func(clientID int) {
		client.UpdateChannelName(clientName)
	})
	s := &Streamer{client: client}
	go s.stream()
	return s, nil
}

func (s *Streamer) SendChat(text string) {
	s.client.SendChatMessage(text)
}

func (s *Streamer) SetOnChatReceived(callback func(string)) {
	s.client.SetOnChatReceived(callback)
}

func (s *Streamer) Close() error {
	s.lock.Lock()
	if s.file != nil {
		s.file.Close()
		s.file = nil
		s.decoder = nil
		s.chatMessages = nil
	}
	s.closed = true
	s.lock.Unlock()
	time.Sleep(100 * time.Millisecond)
	return nil
}

func (s *Streamer) StopStream() {
	s.lock.Lock()
	defer s.lock.Unlock()
	if s.file != nil {
		s.file.Close()
		s.file = nil
		s.decoder = nil
		s.chatMessages = nil
	}
}

func (s *Streamer) Stream(filename string, chatMessages []ChatMessage, offset time.Duration) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	decoder, err := mp3.NewDecoder(file)
	if err != nil {
		return err
	}

	//... seek decoder to offset

	s.lock.Lock()
	defer s.lock.Unlock()
	if s.file != nil {
		s.file.Close()
	}
	s.file = file
	s.decoder = decoder
	s.chatMessages = chatMessages
	s.t0 = time.Now().Add(20 * time.Millisecond)
	s.t = s.t0
	return nil
}

func (s *Streamer) stream() {
	const channels = 2
	const sampleRate = 48000
	const samplesPerPacket = 128

	// At nanosecond resolution, with a sample rate of 48000,
	// this will send packets 1 microsecond too early per 4 seconds
	// so after 4 minutes, the packets will be sent 60 microseconds
	// too early, which should not overflow the buffers.
	// Even 600 microseconds too early should not overflow the
	// buffers, and I do not see using this to play 40 minute or
	// longer audio files.
	dt := time.Second * samplesPerPacket / sampleRate

	buf := make([]byte, 8192)
	var decodedFrames []int16
	resampledFrames := make([]int16, sampleRate*channels)
	currentFrameIndex := 0

	for {
		var closed bool
		var file *os.File
		var decoder *mp3.Decoder
		var chatMessages []ChatMessage
		var t0, t time.Time
		func() {
			s.lock.Lock()
			defer s.lock.Unlock()
			closed = s.closed
			file = s.file
			decoder = s.decoder
			chatMessages = s.chatMessages
			t0 = s.t0
			t = s.t
		}()
		if closed {
			s.client.Close()
			return
		}
		if decoder == nil {
			clear(resampledFrames[:samplesPerPacket*channels])
			time.Sleep(dt)
			if err := s.client.SendRawAudioFrame(resampledFrames[:samplesPerPacket*channels]); err != nil {
				panic(err.Error())
			}
			continue
		}
		if currentFrameIndex == 0 || currentFrameIndex >= sampleRate*channels {
			currentFrameIndex = 0
			decodedFramesSize := decoder.SampleRate() * channels
			if len(decodedFrames) < decodedFramesSize {
				decodedFrames = make([]int16, decodedFramesSize)
			}
			count := 0
			for count < decodedFramesSize {
				n, err := decoder.Read(buf[:min(len(buf), 2*(decodedFramesSize-count))])
				if err != nil && err != io.EOF {
					panic(err.Error())
				}
				if n%2 != 0 {
					panic("?")
				}
				if n == 0 {
					break
				}
				for i := range n / 2 {
					decodedFrames[count+i] = int16(buf[2*i]) | (int16(buf[2*i+1]) << 8)
				}
				count += n / 2
			}
			if count == 0 {
				file.Close()
				func() {
					s.lock.Lock()
					defer s.lock.Unlock()
					if s.decoder == decoder && s.t0 == t0 {
						s.file = nil
						s.decoder = nil
						s.chatMessages = nil
						s.t0 = time.Time{}
					}
				}()
				continue
			}
			if count < len(decodedFrames) {
				clear(decodedFrames[count:])
			}
			n, err := jamulus.ResampleLinear(decodedFrames[:decodedFramesSize], decoder.SampleRate(), sampleRate, channels, resampledFrames)
			if err != nil {
				panic(err.Error())
			}
			if n < len(resampledFrames) {
				clear(resampledFrames[n:])
			}
		}
		time.Sleep(t.Sub(time.Now()))
		t = t.Add(dt)
		if err := s.client.SendRawAudioFrame(resampledFrames[currentFrameIndex : currentFrameIndex+samplesPerPacket*channels]); err != nil {
			panic(err.Error())
		}
		currentFrameIndex += samplesPerPacket * channels
		for len(chatMessages) > 0 && t0.Add(chatMessages[0].dt).After(t) {
			s.client.SendChatMessage(chatMessages[0].message)
			chatMessages = chatMessages[1:]
		}
		func() {
			s.lock.Lock()
			defer s.lock.Unlock()
			if s.decoder == decoder && s.t0 == t0 {
				s.t = t
				s.chatMessages = chatMessages
			}
		}()
	}
}
