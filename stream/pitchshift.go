package stream

import (
	"backjam/jamulus"
)

const (
	chunkSize = jamulus.HopSize
)

type pitchshiftStream struct {
	stream Stream

	buffer [chunkSize][Channels]float64
	index  int
	count  int

	pitchShifter *jamulus.MultiChannelPhaseVocoder
}

func PitchshiftStream(factor float64, stream Stream) Stream {
	if factor == 0 || factor == 1 {
		return stream
	}
	s := &pitchshiftStream{
		stream:       stream,
		pitchShifter: jamulus.NewMultiChannelPhaseVocoder(Channels, factor),
	}
	return s
}

func (s *pitchshiftStream) Close() {
	s.stream.Close()
}

func (s *pitchshiftStream) Done() bool {
	return s.index >= s.count && s.stream.Done()
}

func (s *pitchshiftStream) Read(buffer [][Channels]float64) (int, error) {
	for i := range buffer {
		if s.index >= s.count {
			n, err := s.stream.Read(s.buffer[:])
			if err != nil {
				return 0, err
			}
			s.index = 0
			s.count = n
			clear(s.buffer[n:])
			s.pitchShifter.ProcessInterleavedChunk(s.buffer[:])
		}
		if s.index >= s.count {
			return i, nil
		}
		buffer[i] = s.buffer[s.index]
		s.index++
	}
	return len(buffer), nil
}

func (s *pitchshiftStream) SampleRate() int {
	return s.stream.SampleRate()
}
