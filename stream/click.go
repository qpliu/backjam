package stream

import (
	"math"
	"time"
)

type clickStream struct {
	stream        Stream
	offset        int
	clickInterval int
	clickCount    int
	sampleRate    int
	index         int
	clickIndex    int
	clickSamples  []float64
}

const (
	clickDuration  = 40 * time.Millisecond
	clickAttack    = time.Millisecond / 2
	clickDecay     = 10 * time.Millisecond
	clickFrequency = 1000
)

func ClickStream(offset, clickInterval time.Duration, clickCount, sampleRate int, stream Stream) Stream {
	if clickCount <= 0 {
		return stream
	}
	dt := time.Second / time.Duration(sampleRate)
	clickSamples := make([]float64, int(float64(clickDuration)/float64(dt)))
	for i := range clickSamples {
		t := time.Duration(i) * dt
		var a float64
		omega := 2 * math.Pi / float64(time.Second) * clickFrequency
		if t <= clickAttack {
			a = float64(t) / float64(clickAttack)
		} else {
			a = math.Exp(-float64(t-clickAttack) / float64(clickDecay))
		}
		clickSamples[i] = 0.7 * 32767 * a * math.Sin(omega*float64(t))
	}
	return &clickStream{
		stream:        stream,
		offset:        int(float64(offset) * float64(sampleRate) / float64(time.Second)),
		clickInterval: int(float64(clickInterval) * float64(sampleRate) / float64(time.Second)),
		clickCount:    clickCount,
		sampleRate:    sampleRate,
		index:         0,
		clickIndex:    0,
		clickSamples:  clickSamples,
	}
}

func (s *clickStream) Close() {
	s.stream.Close()
}

func (s *clickStream) Done() bool {
	return s.stream.Done()
}

func (s *clickStream) Read(buffer [][Channels]float64) (int, error) {
	index := s.index
	var count int
	clear(buffer)
	if index >= s.offset {
		n, err := s.stream.Read(buffer)
		if err != nil {
			return n, err
		}
		count = n
	} else if s.offset-index < len(buffer) {
		n, err := s.stream.Read(buffer[s.offset-index:])
		if err != nil {
			return n, err
		}
		count = s.offset - index + n
	} else {
		count = len(buffer)
	}
	if index >= s.offset && s.clickIndex >= s.clickCount {
		return count, nil
	}
	s.index = index + count
	for s.clickIndex*s.clickInterval < s.index && s.clickIndex < s.clickCount {
		if (s.clickIndex+1)*s.clickInterval < index {
			s.clickIndex++
			continue
		}
		for i, clickSample := range s.clickSamples {
			j := s.clickIndex*s.clickInterval + i - index
			if j >= 0 && j < count {
				for ch := range Channels {
					buffer[j][ch] += clickSample
				}
			}
		}
		if (s.clickIndex+1)*s.clickInterval < s.index {
			s.clickIndex++
		} else {
			break
		}
	}
	return count, nil
}

func (s *clickStream) SampleRate() int {
	return s.sampleRate
}

func (s *clickStream) SetVolume(volume int) {
	s.stream.SetVolume(volume)
}

func (s *clickStream) SetStemVolume(tag string, volume int) {
	s.stream.SetStemVolume(tag, volume)
}
