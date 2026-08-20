package stream

const Channels = 2

type Stream interface {
	Close()
	Done() bool
	Read([][Channels]float64) (int, error)
	SampleRate() int
	SetVolume(int)
	SetStemVolume(string, int)
}
