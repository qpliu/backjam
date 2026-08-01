package main

import (
	"math"
	"sync"
	"time"

	"backjam/jamulus"
	"backjam/stream"
)

const (
	Channels         = stream.Channels
	SampleRate       = 48000
	SamplesPerPacket = 128
)

type Streamer struct {
	client *jamulus.Client
	lock   sync.Mutex
	closed bool

	streamPacketizer stream.StreamPacketizer
	t0               time.Time
	t                time.Time
	chatMessages     []ChatMessage
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
	s := &Streamer{
		client: client,
		t:      time.Now(),
	}
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
	s.streamPacketizer.Stream(nil)
	s.chatMessages = nil
	s.closed = true
	s.lock.Unlock()
	time.Sleep(100 * time.Millisecond)
	return nil
}

func (s *Streamer) StopStream() {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.streamPacketizer.Stream(nil)
	s.chatMessages = nil
}

func (s *Streamer) Stream(file *File, chatMessages []ChatMessage, offset time.Duration, volume, pitchShift, speed int, stemVolumes []int) error {
	var str stream.Stream
	if len(file.Stems) == 0 {
		str1, err := stream.MP3Stream(file.GetAudioFileName(), offset)
		if err != nil {
			return err
		}
		str = str1
	} else {
		streams := make([]stream.Stream, len(file.Stems))
		for i := range file.Stems {
			str1, err := stream.MP3Stream(file.GetStemFileName(i), offset)
			if err != nil {
				return err
			}
			streams[i] = str1
		}
		str = stream.MixerStream(streams, stemVolumes)
	}
	if speed == 0 || speed == 100 {
		if pitchShift != 0 {
			str = stream.PitchshiftStream(math.Pow(2, float64(pitchShift)/1200), str)
		}
		str = stream.ResampleStream(SampleRate, str)
	} else {
		if pitchShift == 0 {
			str = stream.PitchshiftStream(100/float64(speed), str)
		} else {
			str = stream.PitchshiftStream(100/float64(speed)*math.Pow(2, float64(pitchShift)/1200), str)
		}
		str = stream.ResampleStream(int(float64(SampleRate)*100/float64(speed)), str)
	}
	str = stream.VolumeStream(volume, str)

	for len(chatMessages) > 0 && chatMessages[0].dt < offset {
		chatMessages = chatMessages[1:]
	}
	for i := range chatMessages {
		chatMessages[i].dt -= offset
		if speed != 0 && speed != 100 {
			chatMessages[i].dt = time.Duration(float64(chatMessages[i].dt) * 100 / float64(speed))
		}
	}

	s.lock.Lock()
	defer s.lock.Unlock()
	s.streamPacketizer.Stream(str)
	s.chatMessages = chatMessages
	s.t0 = time.Now().Add(20 * time.Millisecond)
	s.t = s.t0
	return nil
}

func (s *Streamer) stream() {
	// At nanosecond resolution, with a sample rate of 48000,
	// this will send packets 1 microsecond too early per 4 seconds
	// so after 4 minutes, the packets will be sent 60 microseconds
	// too early, which should not overflow the buffers.
	// Even 600 microseconds too early should not overflow the
	// buffers, and I do not see using this to play 40 minute or
	// longer audio files.
	dt := time.Second * SamplesPerPacket / SampleRate
	frameBuffer := make([]int16, SamplesPerPacket*Channels)

	for {
		var closed bool
		var chatMessages []ChatMessage
		var t0, t time.Time
		func() {
			s.lock.Lock()
			defer s.lock.Unlock()
			closed = s.closed
			if s.streamPacketizer.Done() {
				s.chatMessages = nil
				s.t0 = time.Time{}
			}
			t0 = s.t0
			t = s.t
			s.t = t.Add(dt)
			if err := s.streamPacketizer.NextFrame(frameBuffer); err != nil {
				panic(err.Error())
			}
			for len(s.chatMessages) > 0 && t.After(t0.Add(s.chatMessages[0].dt)) {
				if s.chatMessages[0].message != "" {
					chatMessages = append(chatMessages, s.chatMessages[0])
				}
				s.chatMessages = s.chatMessages[1:]
			}
		}()
		if closed {
			s.client.Close()
			return
		}
		time.Sleep(t.Sub(time.Now()))
		if err := s.client.SendRawAudioFrame(frameBuffer); err != nil {
			panic(err.Error())
		}
		for _, chatMessage := range chatMessages {
			s.client.SendChatMessage(chatMessage.message)
		}
	}
}
