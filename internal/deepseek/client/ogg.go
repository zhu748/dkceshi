package client

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
)

// Ogg Opus container constants. DeepSeek returns bare Opus packets whose TOC
// byte is 0x6b (config 13: Hybrid SWB, 20 ms, mono), so the container is built
// around 48 kHz mono. The TOC is still parsed per packet so a different frame
// size would not corrupt granule positions.
const (
	opusSampleRate = 48000
	opusChannels   = 1
	// 312 samples (6.5 ms) is libopus' default encoder delay. Players subtract
	// it from the first granule position.
	opusPreSkip = 312
)

// EncodeOggOpus wraps bare Opus packets into an Ogg Opus stream. Each packet
// gets its own page, which keeps granule bookkeeping trivial and is accepted
// by every Ogg demuxer.
func EncodeOggOpus(frames [][]byte) ([]byte, error) {
	packets := make([][]byte, 0, len(frames))
	for _, frame := range frames {
		if len(frame) > 0 {
			packets = append(packets, frame)
		}
	}
	if len(packets) == 0 {
		return nil, errors.New("no opus frames")
	}

	var serialBytes [4]byte
	if _, err := rand.Read(serialBytes[:]); err != nil {
		return nil, err
	}
	serial := binary.LittleEndian.Uint32(serialBytes[:])
	if serial == 0 {
		serial = 1
	}

	var out bytes.Buffer
	var seq uint32

	head := make([]byte, 19)
	copy(head, "OpusHead")
	head[8] = 1 // version
	head[9] = opusChannels
	binary.LittleEndian.PutUint16(head[10:], opusPreSkip)
	binary.LittleEndian.PutUint32(head[12:], opusSampleRate)
	binary.LittleEndian.PutUint16(head[16:], 0) // output gain
	head[18] = 0                                // channel mapping family
	bosPage, err := oggPage(serial, seq, 0x02, 0, [][]byte{head})
	if err != nil {
		return nil, err
	}
	out.Write(bosPage) // BOS
	seq++

	var tags bytes.Buffer
	tags.WriteString("OpusTags")
	vendor := []byte("ds2api")
	_ = binary.Write(&tags, binary.LittleEndian, uint32(len(vendor)))
	tags.Write(vendor)
	_ = binary.Write(&tags, binary.LittleEndian, uint32(0))
	tagsPage, err := oggPage(serial, seq, 0x00, 0, [][]byte{tags.Bytes()})
	if err != nil {
		return nil, err
	}
	out.Write(tagsPage)
	seq++

	granule := uint64(opusPreSkip)
	// One packet per page is the most conservative layout: Android's
	// ExoPlayer OggExtractor rejects the same stream when several packets share
	// a page, while every player accepts this form.
	for i, packet := range packets {
		granule += uint64(opusPacketSamples(packet))
		headerType := byte(0)
		if i == len(packets)-1 {
			headerType = 0x04 // EOS
		}
		dataPage, err := oggPage(serial, seq, headerType, granule, [][]byte{packet})
		if err != nil {
			return nil, err
		}
		out.Write(dataPage)
		seq++
	}
	return out.Bytes(), nil
}

// opusPacketSamples returns the packet duration in 48 kHz samples. A TOC
// config gives the duration of one frame; code 1/2 carry two frames and code 3
// carries 1-48 frames whose count lives in the second byte. DeepSeek's packets
// use code 3 with five 20 ms frames, so ignoring the frame count made the Ogg
// granule (and the player's duration) five times too short.
func opusPacketSamples(packet []byte) int {
	if len(packet) == 0 {
		return 0
	}
	toc := packet[0]
	samplesPerFrame := opusFrameSamples(int(toc >> 3))
	frameCount := 1
	switch int(toc & 0x03) {
	case 1, 2:
		frameCount = 2
	case 3:
		if len(packet) > 1 {
			frameCount = int(packet[1] & 0x3F)
			if frameCount == 0 {
				frameCount = 1
			}
		}
	}
	return frameCount * samplesPerFrame
}

// opusFrameSamples returns one frame's duration in 48 kHz samples, derived from
// the TOC configuration bits (RFC 6716 §3.1).
func opusFrameSamples(config int) int {
	switch {
	case config < 12: // SILK
		switch config & 3 {
		case 0:
			return 480 // 10 ms
		case 1:
			return 960 // 20 ms
		case 2:
			return 1920 // 40 ms
		default:
			return 2880 // 60 ms
		}
	case config < 16: // Hybrid
		if config&1 == 0 {
			return 480 // 10 ms
		}
		return 960 // 20 ms
	default: // CELT
		switch config & 3 {
		case 0:
			return 120 // 2.5 ms
		case 1:
			return 240 // 5 ms
		case 2:
			return 480 // 10 ms
		default:
			return 960 // 20 ms
		}
	}
}

// oggPage builds one Ogg page. packets must fit in a single page (the lacing
// table allows at most 255 segments); Opus packets are far below that limit.
func oggPage(serial, seq uint32, headerType byte, granule uint64, packets [][]byte) ([]byte, error) {
	lacing := make([]byte, 0, len(packets))
	dataLen := 0
	for _, packet := range packets {
		remaining := len(packet)
		dataLen += remaining
		for remaining >= 255 {
			lacing = append(lacing, 255)
			remaining -= 255
		}
		lacing = append(lacing, byte(remaining))
	}
	if len(lacing) > 255 {
		// Callers never hit this with Opus packets; keep the failure loud
		// rather than emitting a corrupt page.
		return nil, errors.New("ogg packet exceeds maximum 255 lacing segments")
	}

	page := make([]byte, 27+len(lacing)+dataLen)
	copy(page[0:4], "OggS")
	page[4] = 0
	page[5] = headerType
	binary.LittleEndian.PutUint64(page[6:], granule)
	binary.LittleEndian.PutUint32(page[14:], serial)
	binary.LittleEndian.PutUint32(page[18:], seq)
	// CRC field (22:26) stays zero while the checksum is computed.
	page[26] = byte(len(lacing))
	copy(page[27:], lacing)
	offset := 27 + len(lacing)
	for _, packet := range packets {
		copy(page[offset:], packet)
		offset += len(packet)
	}
	binary.LittleEndian.PutUint32(page[22:], oggCRC(page))
	return page, nil
}

var oggCRCTable [256]uint32

func init() {
	for i := 0; i < 256; i++ {
		r := uint32(i) << 24
		for j := 0; j < 8; j++ {
			if r&0x80000000 != 0 {
				r = (r << 1) ^ 0x04c11db7
			} else {
				r <<= 1
			}
		}
		oggCRCTable[i] = r
	}
}

// oggCRC implements the Ogg page checksum: CRC-32 with polynomial 0x04c11db7,
// initial value 0, no reflection and no final XOR.
func oggCRC(data []byte) uint32 {
	var crc uint32
	for _, b := range data {
		crc = (crc << 8) ^ oggCRCTable[byte(crc>>24)^b]
	}
	return crc
}
