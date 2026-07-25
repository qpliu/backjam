package stream

type volumeStream struct {
	volume float64
	stream Stream
}

func VolumeStream(volume int, stream Stream) Stream {
	if volume == 0 || volume == 100 {
		return stream
	}
	return &volumeStream{float64(volume) / 100, stream}
}

func (s *volumeStream) Close() {
	s.stream.Close()
}

func (s *volumeStream) Done() bool {
	return s.stream.Done()
}

func (s *volumeStream) Read(buffer [][Channels]float64) (int, error) {
	n, err := s.stream.Read(buffer)
	if err != nil {
		return n, err
	}
	for i := range buffer {
		for ch := range Channels {
			buffer[i][ch] *= s.volume
		}
	}
	return n, nil
}

func (s *volumeStream) SampleRate() int {
	return s.stream.SampleRate()
}
