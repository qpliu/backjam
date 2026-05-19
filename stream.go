package main

import (
	"io"
	"time"

	"github.com/hajimehoshi/go-mp3"
	"github.com/hraban/opus"
)

func StreamMP3(r io.Reader, client *JamulusClient) {
	decoder, err := mp3.NewDecoder(r)
	if err != nil {
		panic(err.Error())
	}
	sampleRate := decoder.SampleRate()

	encoder, err := opus.NewEncoder(sampleRate, 2, opus.AppAudio)
	if err != nil {
		panic(err.Error())
	}

	frameSizeMs := 20
	frameSizeBytes := sampleRate / 1000 * frameSizeMs * 2 * 2

	buf := make([]byte, frameSizeBytes)
	frame := make([]int64, frameSizeBytes/2)
	t := time.Now().Add(frameSizeMs * time.Millisecond)
	for {
		n, err := decoder.Read(buf)
		if err != nil && err != io.EOF {
			panic(err.Error())
		}
		if n == 0 {
			break
		}
		for i := range n / 2 {
			frame[i] = int16(buf[i*2]) | (int16(buf[i*2+1]) << 8)
		}
		opusFrame, err := encoder.encode(frame)
		if err != nil {
			panic(err.Error())
		}
		time.Sleep(t.Sub(time.Now()))
		t = t.Add(frameSizeMs * time.Millisecond)

		if err := client.SendOpusFrame(opusFrame); err != nil {
			panic(err.Error())
		}
	}
}
