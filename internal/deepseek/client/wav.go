package client

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/pion/opus"
)

const (
	wavSampleRate = 48000
	wavChannels   = 1
	wavBits       = 16
)

// EncodeWAVFromOpus decodes bare Opus packets into a 16-bit PCM WAV file.
// Android's ExoPlayer (used by reader apps such as Legado) does not reliably
// recognize the Ogg Opus stream, so the GET endpoint offers WAV as well.
func EncodeWAVFromOpus(frames [][]byte) ([]byte, error) {
	decoder, err := opus.NewDecoderWithOutput(wavSampleRate, wavChannels)
	if err != nil {
		return nil, fmt.Errorf("opus decoder: %w", err)
	}
	// One Opus packet carries at most 960 ms of audio (RFC 6716: TOC code 3 carries
	// up to 48 frames of 20 ms each). Allocate for the theoretical maximum, and
	// expand dynamically if an oversized packet is encountered.
	const maxOpusPacketSamples = wavSampleRate * 960 / 1000
	out := make([]int16, maxOpusPacketSamples)
	var pcm []byte
	for _, packet := range frames {
		if len(packet) == 0 {
			continue
		}
		if needed := opusPacketSamples(packet); needed > len(out) {
			out = make([]int16, needed)
		}
		samples, err := decoder.DecodeToInt16(packet, out)
		if err != nil {
			return nil, fmt.Errorf("opus decode: %w", err)
		}
		for i := 0; i < samples; i++ {
			pcm = binary.LittleEndian.AppendUint16(pcm, uint16(out[i]))
		}
	}
	if len(pcm) == 0 {
		return nil, errors.New("no pcm samples")
	}
	return wavContainer(pcm), nil
}

func wavContainer(pcm []byte) []byte {
	const headerSize = 44
	dataSize := len(pcm)
	blockAlign := wavChannels * wavBits / 8
	byteRate := wavSampleRate * blockAlign
	out := make([]byte, headerSize+dataSize)
	copy(out[0:4], "RIFF")
	binary.LittleEndian.PutUint32(out[4:], uint32(36+dataSize))
	copy(out[8:12], "WAVE")
	copy(out[12:16], "fmt ")
	binary.LittleEndian.PutUint32(out[16:], 16)
	binary.LittleEndian.PutUint16(out[20:], 1) // PCM
	binary.LittleEndian.PutUint16(out[22:], wavChannels)
	binary.LittleEndian.PutUint32(out[24:], wavSampleRate)
	binary.LittleEndian.PutUint32(out[28:], uint32(byteRate))
	binary.LittleEndian.PutUint16(out[32:], uint16(blockAlign))
	binary.LittleEndian.PutUint16(out[34:], wavBits)
	copy(out[36:40], "data")
	binary.LittleEndian.PutUint32(out[40:], uint32(dataSize))
	copy(out[headerSize:], pcm)
	return out
}
