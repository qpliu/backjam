package main

import (
	"io"
	"time"

	"github.com/hajimehoshi/go-mp3"

	"backjam/jamulus"
)

type ChatMessage struct {
	dt      time.Duration
	message string
}

func StreamMP3(r io.Reader, chats []ChatMessage, client *jamulus.Client) {
	decoder, err := mp3.NewDecoder(r)
	if err != nil {
		panic(err.Error())
	}
	decodeSampleRate := decoder.SampleRate()

	const channels = 2
	const sampleRate = 48000
	const samplesPerPacket = 128

	decodeFrameSize := decodeSampleRate * channels
	resampleFrameSize := sampleRate * channels

	// At nanosecond resolution, with a sample rate of 48000,
	// this will send packets 1 microsecond too early per 4 seconds
	// so after 4 minutes, the packets will be sent 60 microseconds too
	// early, which should not overflow the buffers.
	dt := time.Second * samplesPerPacket / sampleRate

	buf := make([]byte, 2*decodeFrameSize)
	decodeFrame := make([]int16, decodeFrameSize)
	resampleFrame := make([]int16, resampleFrameSize)
	t0 := time.Now().Add(100 * time.Millisecond)
	t := t0
	for {
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
		n, err := jamulus.ResampleLinear(decodeFrame, decodeSampleRate, sampleRate, channels, resampleFrame)
		if err != nil {
			panic(err.Error())
		}
		if n < resampleFrameSize {
			clear(resampleFrame[n:])
		}
		for i := 0; i < resampleFrameSize; i += samplesPerPacket * channels {
			time.Sleep(t.Sub(time.Now()))
			t = t.Add(dt)

			if err := client.SendRawAudioFrame(resampleFrame[i : i+samplesPerPacket*channels]); err != nil {
				panic(err.Error())
			}
			if len(chats) > 0 && time.Now().After(t0.Add(chats[0].dt)) {
				client.SendChatMessage(chats[0].message)
				chats = chats[1:]
			}
		}
	}
}
