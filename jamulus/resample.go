package jamulus

import (
	"fmt"
	"math"
)

// ResampleMethod specifies the resampling algorithm
type ResampleMethod int

const (
	MethodLinear ResampleMethod = iota
	MethodSinc
	MethodBandlimited
)

// ResampleConfig holds configuration for resampling
type ResampleConfig struct {
	Method    ResampleMethod
	FilterLen int // Length of the sinc filter kernel
	Quality   int // 0=low, 1=medium, 2=high
}

// Resample resamples PCM data from one sample rate to another.
// The output array must be pre-allocated with sufficient capacity.
// Returns the number of samples written to the output array.
func Resample(input []int16, inputRate, outputRate, channels int, output []int16) (int, error) {
	config := &ResampleConfig{
		Method:    MethodBandlimited,
		FilterLen: 64,
		Quality:   1,
	}
	return ResampleWithConfig(input, inputRate, outputRate, channels, output, config)
}

// ResampleLinear performs linear interpolation resampling.
// The output array must be pre-allocated with sufficient capacity.
// Returns the number of samples written to the output array.
func ResampleLinear(input []int16, inputRate, outputRate, channels int, output []int16) (int, error) {
	if inputRate == outputRate {
		// No resampling needed
		n := copy(output, input)
		return n, nil
	}

	if len(input) == 0 {
		return 0, nil
	}

	if inputRate <= 0 || outputRate <= 0 {
		return 0, fmt.Errorf("sample rates must be positive")
	}

	if channels != 1 && channels != 2 {
		return 0, fmt.Errorf("only mono (1) and stereo (2) channels supported")
	}

	ratio := float64(inputRate) / float64(outputRate)
	numSamples := len(input) / channels
	outputSamples := int(math.Ceil(float64(numSamples) / ratio))
	maxOutputSamples := len(output) / channels

	if outputSamples > maxOutputSamples {
		return 0, fmt.Errorf("output array too small: need %d samples, have capacity for %d",
			outputSamples, maxOutputSamples)
	}

	outPos := 0
	for i := 0; i < outputSamples; i++ {
		inputPos := float64(i) * ratio

		for ch := 0; ch < channels; ch++ {
			idx := int(inputPos) * channels
			frac := inputPos - float64(int(inputPos))

			if idx+channels >= len(input) {
				// Use last sample
				output[outPos] = input[len(input)-channels+ch]
			} else {
				// Linear interpolation
				sample1 := float64(input[idx+ch])
				sample2 := float64(input[idx+channels+ch])
				interpolated := sample1 + frac*(sample2-sample1)
				output[outPos] = int16(clamp(interpolated, -32768, 32767))
			}
			outPos++
		}
	}

	return outPos, nil
}

// ResampleWithConfig performs resampling with custom configuration.
// The output array must be pre-allocated with sufficient capacity.
// Returns the number of samples written to the output array.
func ResampleWithConfig(input []int16, inputRate, outputRate, channels int,
	output []int16, config *ResampleConfig) (int, error) {

	if inputRate == outputRate {
		n := copy(output, input)
		return n, nil
	}

	if len(input) == 0 {
		return 0, nil
	}

	if inputRate <= 0 || outputRate <= 0 {
		return 0, fmt.Errorf("sample rates must be positive")
	}

	if channels != 1 && channels != 2 {
		return 0, fmt.Errorf("only mono (1) and stereo (2) channels supported")
	}

	switch config.Method {
	case MethodLinear:
		return ResampleLinear(input, inputRate, outputRate, channels, output)
	case MethodSinc:
		return resampleSinc(input, inputRate, outputRate, channels, output, config)
	case MethodBandlimited:
		return resampleBandlimited(input, inputRate, outputRate, channels, output, config)
	default:
		return 0, fmt.Errorf("unknown resampling method")
	}
}

// resampleSinc performs windowed-sinc interpolation resampling.
func resampleSinc(input []int16, inputRate, outputRate, channels int,
	output []int16, config *ResampleConfig) (int, error) {

	ratio := float64(inputRate) / float64(outputRate)
	numSamples := len(input) / channels
	outputSamples := int(math.Ceil(float64(numSamples) / ratio))
	maxOutputSamples := len(output) / channels

	if outputSamples > maxOutputSamples {
		return 0, fmt.Errorf("output array too small: need %d samples, have capacity for %d",
			outputSamples, maxOutputSamples)
	}

	filterLen := config.FilterLen
	if filterLen == 0 {
		filterLen = 64
	}

	// Create sinc filter kernel (windowed)
	kernel := createSincKernel(filterLen)

	outPos := 0
	for i := 0; i < outputSamples; i++ {
		inputPos := float64(i) * ratio

		for ch := 0; ch < channels; ch++ {
			sample := 0.0
			centerIdx := int(inputPos)

			// Apply sinc filter
			for k := 0; k < filterLen; k++ {
				idx := centerIdx - filterLen/2 + k
				if idx >= 0 && idx*channels+ch < len(input) {
					frac := inputPos - float64(centerIdx)
					sample += float64(input[idx*channels+ch]) * sinc(float64(k-filterLen/2)-frac) * kernel[k]
				}
			}

			output[outPos] = int16(clamp(sample, -32768, 32767))
			outPos++
		}
	}

	return outPos, nil
}

// resampleBandlimited performs band-limited polyphase resampling.
func resampleBandlimited(input []int16, inputRate, outputRate, channels int,
	output []int16, config *ResampleConfig) (int, error) {

	ratio := float64(inputRate) / float64(outputRate)
	numSamples := len(input) / channels
	outputSamples := int(math.Ceil(float64(numSamples) / ratio))
	maxOutputSamples := len(output) / channels

	if outputSamples > maxOutputSamples {
		return 0, fmt.Errorf("output array too small: need %d samples, have capacity for %d",
			outputSamples, maxOutputSamples)
	}

	// Use polyphase filtering for rational resampling
	filterLen := config.FilterLen
	if filterLen == 0 {
		filterLen = 64
	}

	kernel := createSincKernel(filterLen)

	outPos := 0
	inputIdx := 0.0

	for i := 0; i < outputSamples; i++ {
		for ch := 0; ch < channels; ch++ {
			sample := 0.0
			centerIdx := int(inputIdx)

			// Apply polyphase filter
			for k := 0; k < filterLen; k++ {
				idx := centerIdx - filterLen/2 + k
				if idx >= 0 && idx*channels+ch < len(input) {
					frac := inputIdx - float64(centerIdx)
					sincVal := sinc(float64(k-filterLen/2) - frac)
					sample += float64(input[idx*channels+ch]) * sincVal * kernel[k]
				}
			}

			output[outPos] = int16(clamp(sample, -32768, 32767))
			outPos++
		}

		inputIdx += ratio
	}

	return outPos, nil
}

// Helper functions

// CalculateOutputSize calculates the number of output samples needed.
// Useful for pre-allocating the output array.
func CalculateOutputSize(inputSize, inputRate, outputRate, channels int) int {
	if inputRate == outputRate {
		return inputSize
	}
	numSamples := inputSize / channels
	outputSamples := int(math.Ceil(float64(numSamples) * float64(outputRate) / float64(inputRate)))
	return outputSamples * channels
}

// sinc calculates the sinc function value
func sinc(x float64) float64 {
	if x == 0 {
		return 1.0
	}
	px := math.Pi * x
	return math.Sin(px) / px
}

// createSincKernel creates a windowed sinc filter kernel
func createSincKernel(length int) []float64 {
	kernel := make([]float64, length)
	for i := 0; i < length; i++ {
		// Hann window
		window := 0.5 * (1 - math.Cos(2*math.Pi*float64(i)/float64(length-1)))
		kernel[i] = window
	}
	return kernel
}

// clamp clamps a value between min and max
func clamp(val, min, max float64) float64 {
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}

// gcdInt calculates the greatest common divisor of two integers
func gcdInt(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}
