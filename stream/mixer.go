package stream

const (
	mixerBufferSize = 1024
)

type mixerStream struct {
	streams []Stream
	tags    []string
	volumes []float64

	buffer [][mixerBufferSize][Channels]float64
	index  int
	count  int
}

func MixerStream(streams []Stream, tags []string, volumes []int) Stream {
	vols := make([]float64, len(streams))
	for i := range vols {
		if i < len(volumes) {
			vols[i] = float64(volumes[i]) / 100
		} else {
			vols[i] = 1
		}
	}
	return &mixerStream{
		streams: streams,
		tags:    tags,
		volumes: vols,
		buffer:  make([][mixerBufferSize][Channels]float64, len(streams)),
	}
}

func (s *mixerStream) Close() {
	for _, str := range s.streams {
		str.Close()
	}
}

func (s *mixerStream) Done() bool {
	return s.index >= s.count && s.streams[0].Done()
}

func (s *mixerStream) Read(buffer [][Channels]float64) (int, error) {
	for i := range buffer {
		if s.index >= s.count {
			for j, str := range s.streams {
				n, err := str.Read(s.buffer[j][:])
				if err != nil {
					return 0, err
				}
				s.index = 0
				s.count = n
				clear(s.buffer[j][n:])
			}
		}
		if s.index >= s.count {
			return i, nil
		}
		clear(buffer[i][:])
		for j := range s.buffer {
			for ch := range Channels {
				buffer[i][ch] += s.volumes[j] * s.buffer[j][s.index][ch]
			}
		}
		s.index++
	}
	return len(buffer), nil
}

func (s *mixerStream) SampleRate() int {
	return s.streams[0].SampleRate()
}

func (s *mixerStream) SetVolume(volume int) {
}

func (s *mixerStream) SetStemVolume(tag string, volume int) {
	for i := range s.tags {
		if tag == s.tags[i] {
			s.volumes[i] = float64(volume) / 100
		}
	}
}
