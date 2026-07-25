package jamulus

import (
	"math"
	"math/cmplx"

	"github.com/argusdusty/gofft"
)

const (
	WindowSize = 1024
	HopSize    = 256 // 75% overlap
)

// ChannelState manages the localized history and phase trackers for a single audio stream channel.
type ChannelState struct {
	inputBuffer        []float32
	outputBuffer       []float32
	prevAnalysisPhase  []float64
	prevSynthesisPhase []float64
}

type MultiChannelPhaseVocoder struct {
	pitch       float64
	numChannels int
	channels    []ChannelState
	windowFunc  []float32

	// Pre-allocated workspace arrays to prevent runtime GC heap allocations
	fftWorkspace   []complex128
	newMagnitudes  []float64
	newFrequencies []float64

	// Ring buffer pointers
	writeIdx int
	readIdx  int
}

// NewMultiChannelPhaseVocoder initializes internal states for N interleaved channels.
func NewMultiChannelPhaseVocoder(numChannels int, pitchFactor float64) *MultiChannelPhaseVocoder {
	pv := &MultiChannelPhaseVocoder{
		pitch:          pitchFactor,
		numChannels:    numChannels,
		channels:       make([]ChannelState, numChannels),
		windowFunc:     make([]float32, WindowSize),
		fftWorkspace:   make([]complex128, WindowSize),
		newMagnitudes:  make([]float64, WindowSize/2+1),
		newFrequencies: make([]float64, WindowSize/2+1),
	}

	for c := 0; c < numChannels; c++ {
		pv.channels[c] = ChannelState{
			inputBuffer:        make([]float32, WindowSize*2),
			outputBuffer:       make([]float32, WindowSize*4),
			prevAnalysisPhase:  make([]float64, WindowSize/2+1),
			prevSynthesisPhase: make([]float64, WindowSize/2+1),
		}
	}

	// Pre-calculate Hann window
	for i := 0; i < WindowSize; i++ {
		pv.windowFunc[i] = float32(0.5 * (1.0 - math.Cos(2.0*math.Pi*float64(i)/float64(WindowSize-1))))
	}
	return pv
}

// ProcessInterleavedChunk processes interleaved PCM16 audio blocks in-place.
// The total slice length must be exactly (HopSize * numChannels).
func (pv *MultiChannelPhaseVocoder) ProcessInterleavedChunk(chunk [][2]float64) {
	expectedLen := HopSize
	if len(chunk) != expectedLen {
		return // Ignore fragments that mismatch configured channel/hop spacing
	}

	// 1. De-interleave and write incoming data into each channel's input history buffer
	for i := 0; i < HopSize; i++ {
		for ch := 0; ch < pv.numChannels; ch++ {
			bufIdx := (pv.writeIdx + i) % len(pv.channels[ch].inputBuffer)
			pv.channels[ch].inputBuffer[bufIdx] = float32(chunk[i][ch]) / 32768.0
		}
	}
	pv.writeIdx = (pv.writeIdx + HopSize) % len(pv.channels[0].inputBuffer)

	// 2. Process each channel independently in the frequency domain
	expect := 2.0 * math.Pi * float64(HopSize) / float64(WindowSize)
	startPos := (pv.writeIdx - WindowSize + len(pv.channels[0].inputBuffer)) % len(pv.channels[0].inputBuffer)

	for ch := 0; ch < pv.numChannels; ch++ {
		cState := &pv.channels[ch]

		// Zero workspaces before starting transformation step
		for i := range pv.newMagnitudes {
			pv.newMagnitudes[i] = 0.0
			pv.newFrequencies[i] = 0.0
		}

		// Apply window function to the history data
		for i := 0; i < WindowSize; i++ {
			sample := cState.inputBuffer[(startPos+i)%len(cState.inputBuffer)]
			pv.fftWorkspace[i] = complex(float64(sample*pv.windowFunc[i]), 0)
		}

		// Forward Time-to-Spectrum conversion
		_ = gofft.FFT(pv.fftWorkspace)

		// Phase Unwrapping & Analysis Loop
		for k := 0; k <= WindowSize/2; k++ {
			c := pv.fftWorkspace[k]
			mag := cmplx.Abs(c)
			phase := math.Atan2(imag(c), real(c))

			tmp := phase - cState.prevAnalysisPhase[k]
			cState.prevAnalysisPhase[k] = phase
			tmp -= float64(k) * expect

			qpd := int(tmp / math.Pi)
			if qpd >= 0 {
				qpd += qpd & 1
			} else {
				qpd -= qpd & 1
			}
			tmp -= math.Pi * float64(qpd)

			tmp = float64(WindowSize) * tmp / (2.0 * math.Pi * float64(HopSize))
			trueFreq := float64(k) + tmp

			// Map and scale pitch into output magnitude/frequency bins
			newK := int(math.Round(float64(k) * pv.pitch))
			if newK <= WindowSize/2 {
				pv.newMagnitudes[newK] += mag
				pv.newFrequencies[newK] = trueFreq * pv.pitch
			}
		}

		// Spectral Synthesis Phase Reconstruction
		pv.fftWorkspace[0] = complex(pv.newMagnitudes[0], 0)
		for k := 1; k <= WindowSize/2; k++ {
			mag := pv.newMagnitudes[k]
			trueFreq := pv.newFrequencies[k]

			tmp := trueFreq - float64(k)
			tmp = 2.0 * math.Pi * tmp * float64(HopSize) / float64(WindowSize)
			tmp += float64(k) * expect

			cState.prevSynthesisPhase[k] += tmp
			phase := cState.prevSynthesisPhase[k]

			pv.fftWorkspace[k] = complex(mag*math.Cos(phase), mag*math.Sin(phase))
			if k < WindowSize/2 {
				pv.fftWorkspace[WindowSize-k] = complex(mag*math.Cos(phase), -mag*math.Sin(phase))
			}
		}
		pv.fftWorkspace[WindowSize/2] = complex(pv.newMagnitudes[WindowSize/2], 0)

		// Inverse Spectrum-to-Time conversion
		_ = gofft.IFFT(pv.fftWorkspace)

		// Add reconstructed block back into channel output buffer
		for i := 0; i < WindowSize; i++ {
			outIdx := (pv.readIdx + i) % len(cState.outputBuffer)
			windowedSample := float32(real(pv.fftWorkspace[i])) * pv.windowFunc[i]
			cState.outputBuffer[outIdx] += windowedSample
		}
	}

	// 3. Normalize channels, re-interleave, and pack values to output chunk
	for i := 0; i < HopSize; i++ {
		outIdx := (pv.readIdx + i) % len(pv.channels[0].outputBuffer)

		for ch := 0; ch < pv.numChannels; ch++ {
			cState := &pv.channels[ch]
			normSample := cState.outputBuffer[outIdx] * 1.0
			cState.outputBuffer[outIdx] = 0.0 // Reset memory slot

			scaled := normSample * 32767.0
			if scaled > 32767.0 {
				scaled = 32767.0
			} else if scaled < -32768.0 {
				scaled = -32768.0
			}

			// Map back to interleaved output indexing configuration
			chunk[i][ch] = float64(scaled)
		}
	}

	pv.readIdx = (pv.readIdx + HopSize) % len(pv.channels[0].outputBuffer)
}
