package main

import (
	"io"
	"time"

	"github.com/hajimehoshi/go-mp3"

	"backjam/jamulus"
)

func StreamMP3(r io.Reader, client *jamulus.Client) {
	decoder, err := mp3.NewDecoder(r)
	if err != nil {
		panic(err.Error())
	}
	decodeSampleRate := decoder.SampleRate()

	const channels = 2
	const sampleRate = 48000
	const frameSizeMs = 20

	decodeFrameSize := decodeSampleRate * frameSizeMs * channels / 1000
	frameSize := sampleRate * frameSizeMs * channels / 1000

	buf := make([]byte, 2*decodeFrameSize)
	decodeFrame := make([]int16, decodeFrameSize)
	frame := make([]int16, frameSize)
	t := time.Now().Add(frameSizeMs * time.Millisecond)
	chatMessages := []string{"5 second", "30 second", "60 second"}
	chatTimes := []time.Time{t.Add(5 * time.Second), t.Add(30 * time.Second), t.Add(60 * time.Second)}
	for j := 0; ; j++ {
		count := 0
		for count < 2*decodeFrameSize {
			n, err := decoder.Read(buf[count:])
			if err != nil && err != io.EOF {
				panic(err.Error())
			}
			if n == 0 {
				break
			}
			count += n
		}
		if count == 0 {
			break
		}
		if count < 2*decodeFrameSize {
			clear(buf[count:])
		}
		for i := range decodeFrameSize {
			decodeFrame[i] = int16(buf[2*i]) | (int16(buf[2*i+1]) << 8)
		}
		n, err := jamulus.ResampleLinear(decodeFrame, decodeSampleRate, sampleRate, channels, frame)
		if err != nil {
			panic(err.Error())
		}
		if n < frameSize {
			clear(frame[n:])
		}
		time.Sleep(t.Sub(time.Now()))
		t = t.Add(frameSizeMs * time.Millisecond)

		if err := client.SendRawAudioFrame(frame); err != nil {
			panic(err.Error())
		}
		if len(chatTimes) > 0 && time.Now().After(chatTimes[0]) {
			client.SendChatMessage(chatMessages[0])
			chatTimes = chatTimes[1:]
			chatMessages = chatMessages[1:]
		}
	}
}
