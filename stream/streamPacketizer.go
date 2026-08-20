package stream

import (
	"fmt"
)

type StreamPacketizer struct {
	stream Stream
	buffer [][Channels]float64
}

func (s *StreamPacketizer) Stream(stream Stream) {
	if s.stream != nil {
		s.stream.Close()
	}
	s.stream = stream
}

func (s *StreamPacketizer) NextFrame(frame []int16) error {
	if s.stream == nil {
		clear(frame)
		return nil
	}
	if s.stream.Done() {
		s.stream.Close()
		s.stream = nil
		clear(frame)
		return nil
	}
	if len(frame)%Channels != 0 {
		return fmt.Errorf("invalid frame length:%d", len(frame))
	}
	n := len(frame) / Channels
	if len(s.buffer) < n {
		s.buffer = make([][Channels]float64, n)
	}
	n, err := s.stream.Read(s.buffer)
	if err != nil {
		return err
	}
	clear(frame[n*Channels:])
	for i := range n {
		for ch := range Channels {
			frame[i*Channels+ch] = int16(max(-32768.0, min(32767.0, s.buffer[i][ch])))
		}
	}
	return nil
}

func (s *StreamPacketizer) SetVolume(volume int) {
	if s.stream != nil {
		s.stream.SetVolume(volume)
	}
}

func (s *StreamPacketizer) SetStemVolume(tag string, volume int) {
	if s.stream != nil {
		s.stream.SetStemVolume(tag, volume)
	}
}

func (s *StreamPacketizer) Done() bool {
	return s.stream == nil
}
