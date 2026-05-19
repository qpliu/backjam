package main

import (
	"os"
)

func main() {
	input := os.Stdin
	client := NewJamulusClient("localhost:22124")
	defer client.Close()

	StreamMP3(input, client)
}
