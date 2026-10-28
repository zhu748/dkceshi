package client

import (
	"encoding/binary"
	"testing"
)

func TestWavContainer(t *testing.T) {
	pcm := make([]byte, 960*2) // 960 16-bit samples = 1920 bytes
	for i := range pcm {
		pcm[i] = byte(i % 256)
	}

	wav := wavContainer(pcm)
	if len(wav) != 44+len(pcm) {
		t.Fatalf("wav length=%d want %d", len(wav), 44+len(pcm))
	}
	if string(wav[0:4]) != "RIFF" {
		t.Fatalf("riff header=%q", string(wav[0:4]))
	}
	riffSize := binary.LittleEndian.Uint32(wav[4:8])
	if riffSize != uint32(36+len(pcm)) {
		t.Fatalf("riffSize=%d want %d", riffSize, 36+len(pcm))
	}
	if string(wav[8:12]) != "WAVE" {
		t.Fatalf("wave header=%q", string(wav[8:12]))
	}
	if string(wav[12:16]) != "fmt " {
		t.Fatalf("fmt header=%q", string(wav[12:16]))
	}
	fmtSize := binary.LittleEndian.Uint32(wav[16:20])
	if fmtSize != 16 {
		t.Fatalf("fmtSize=%d want 16", fmtSize)
	}
	audioFormat := binary.LittleEndian.Uint16(wav[20:22])
	if audioFormat != 1 {
		t.Fatalf("audioFormat=%d want 1 (PCM)", audioFormat)
	}
	numChannels := binary.LittleEndian.Uint16(wav[22:24])
	if numChannels != wavChannels {
		t.Fatalf("numChannels=%d want %d", numChannels, wavChannels)
	}
	sampleRate := binary.LittleEndian.Uint32(wav[24:28])
	if sampleRate != wavSampleRate {
		t.Fatalf("sampleRate=%d want %d", sampleRate, wavSampleRate)
	}
	bitsPerSample := binary.LittleEndian.Uint16(wav[34:36])
	if bitsPerSample != wavBits {
		t.Fatalf("bitsPerSample=%d want %d", bitsPerSample, wavBits)
	}
	if string(wav[36:40]) != "data" {
		t.Fatalf("data chunk=%q", string(wav[36:40]))
	}
	dataSize := binary.LittleEndian.Uint32(wav[40:44])
	if dataSize != uint32(len(pcm)) {
		t.Fatalf("dataSize=%d want %d", dataSize, len(pcm))
	}
}

func TestEncodeWAVFromOpusEmpty(t *testing.T) {
	if _, err := EncodeWAVFromOpus(nil); err == nil {
		t.Fatal("expected an error for nil frames")
	}
	if _, err := EncodeWAVFromOpus([][]byte{{}, {}}); err == nil {
		t.Fatal("expected an error for empty frames")
	}
}

func TestEncodeWAVFromOpusInvalidFrame(t *testing.T) {
	frames := [][]byte{
		{0xff, 0xff},
	}
	if _, err := EncodeWAVFromOpus(frames); err == nil {
		t.Fatal("expected an error for corrupted opus frame")
	}
}
