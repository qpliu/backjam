package stream

type resampleStream struct {
	sampleRate int
	ratio      float64
	t          float64
	stream     Stream
	lastSample [Channels]float64
	sample     [1][Channels]float64
}

func ResampleStream(sampleRate int, stream Stream) Stream {
	if sampleRate == 0 || sampleRate == stream.SampleRate() {
		return stream
	}
	return &resampleStream{
		sampleRate: sampleRate,
		ratio:      float64(stream.SampleRate()) / float64(sampleRate),
		t:          2,
		stream:     stream,
	}
}

func (s *resampleStream) Close() {
	s.stream.Close()
}

func (s *resampleStream) Done() bool {
	return s.stream.Done()
}

func (s *resampleStream) Read(buffer [][Channels]float64) (int, error) {
	for i := range buffer {
		for s.t > 1 {
			s.t -= 1
			s.lastSample = s.sample[0]
			n, err := s.stream.Read(s.sample[:])
			if err != nil {
				return 0, err
			}
			if n == 0 {
				return i, nil
			}
		}
		for ch := range Channels {
			buffer[i][ch] = s.lastSample[ch]*(1-s.t) + s.sample[0][ch]*s.t
		}
		s.t += s.ratio
	}
	return len(buffer), nil
}

func (s *resampleStream) SampleRate() int {
	return s.sampleRate
}
