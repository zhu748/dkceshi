package client

import (
	"bytes"
	"encoding/binary"
	"testing"
)

type parsedOggPage struct {
	headerType byte
	granule    uint64
	serial     uint32
	seq        uint32
	packets    [][]byte
}

func parseOggPages(t *testing.T, data []byte) []parsedOggPage {
	t.Helper()
	var pages []parsedOggPage
	for offset := 0; offset < len(data); {
		if offset+27 > len(data) {
			t.Fatalf("truncated page header at %d", offset)
		}
		if string(data[offset:offset+4]) != "OggS" {
			t.Fatalf("bad capture pattern at %d", offset)
		}
		headerType := data[offset+5]
		granule := binary.LittleEndian.Uint64(data[offset+6:])
		serial := binary.LittleEndian.Uint32(data[offset+14:])
		seq := binary.LittleEndian.Uint32(data[offset+18:])
		storedCRC := binary.LittleEndian.Uint32(data[offset+22:])
		segCount := int(data[offset+26])
		if offset+27+segCount > len(data) {
			t.Fatalf("truncated lacing table at %d", offset)
		}
		lacing := data[offset+27 : offset+27+segCount]
		payloadLen := 0
		for _, v := range lacing {
			payloadLen += int(v)
		}
		pageEnd := offset + 27 + segCount + payloadLen
		if pageEnd > len(data) {
			t.Fatalf("truncated page payload at %d", offset)
		}

		page := append([]byte(nil), data[offset:pageEnd]...)
		for i := 22; i < 26; i++ {
			page[i] = 0
		}
		if got := oggCRC(page); got != storedCRC {
			t.Fatalf("page %d crc=%08x want %08x", len(pages), storedCRC, got)
		}

		var packets [][]byte
		cursor := offset + 27 + segCount
		var current []byte
		for _, v := range lacing {
			current = append(current, data[cursor:cursor+int(v)]...)
			cursor += int(v)
			if v < 255 {
				packets = append(packets, current)
				current = nil
			}
		}
		pages = append(pages, parsedOggPage{
			headerType: headerType,
			granule:    granule,
			serial:     serial,
			seq:        seq,
			packets:    packets,
		})
		offset = pageEnd
	}
	return pages
}

func TestEncodeOggOpus(t *testing.T) {
	frames := [][]byte{
		append([]byte{0x6b, 0x01}, bytes.Repeat([]byte{0x11}, 100)...),
		append([]byte{0x6b, 0x01}, bytes.Repeat([]byte{0x22}, 200)...),
		append([]byte{0x6b, 0x01}, bytes.Repeat([]byte{0x33}, 50)...),
	}
	data, err := EncodeOggOpus(frames)
	if err != nil {
		t.Fatalf("EncodeOggOpus: %v", err)
	}
	pages := parseOggPages(t, data)
	if len(pages) != 5 {
		t.Fatalf("pages=%d want 5 (head + tags + 3 audio)", len(pages))
	}
	if pages[0].headerType != 0x02 {
		t.Fatalf("first page header type=%#x want BOS", pages[0].headerType)
	}
	if len(pages[0].packets) != 1 || string(pages[0].packets[0][:8]) != "OpusHead" {
		t.Fatalf("first packet is not OpusHead: %q", pages[0].packets)
	}
	if pages[0].granule != 0 {
		t.Fatalf("OpusHead granule=%d want 0", pages[0].granule)
	}
	if len(pages[1].packets) != 1 || string(pages[1].packets[0][:8]) != "OpusTags" {
		t.Fatalf("second packet is not OpusTags: %q", pages[1].packets)
	}
	for i, frame := range frames {
		page := pages[2+i]
		if len(page.packets) != 1 || !bytes.Equal(page.packets[0], frame) {
			t.Fatalf("audio page %d payload mismatch", i)
		}
		wantGranule := uint64(opusPreSkip + (i+1)*960)
		if page.granule != wantGranule {
			t.Fatalf("audio page %d granule=%d want %d", i, page.granule, wantGranule)
		}
		if page.serial != pages[0].serial {
			t.Fatalf("audio page %d serial=%d want %d", i, page.serial, pages[0].serial)
		}
	}
	if pages[len(pages)-1].headerType != 0x04 {
		t.Fatalf("last page header type=%#x want EOS", pages[len(pages)-1].headerType)
	}
}

func TestEncodeOggOpusEmpty(t *testing.T) {
	if _, err := EncodeOggOpus(nil); err == nil {
		t.Fatal("expected an error for no frames")
	}
	if _, err := EncodeOggOpus([][]byte{{}, {}}); err == nil {
		t.Fatal("expected an error for empty frames")
	}
}

func TestOpusPacketSamples(t *testing.T) {
	tests := []struct {
		name   string
		packet []byte
		want   int
	}{
		{"silk 10ms", []byte{0x00}, 480},
		{"silk 20ms", []byte{0x08}, 960},
		{"silk 40ms", []byte{0x10}, 1920},
		{"silk 60ms", []byte{0x18}, 2880},
		{"hybrid 10ms", []byte{0x60}, 480},
		{"hybrid 20ms", []byte{0x68}, 960},
		{"code 0 one frame", []byte{0x68}, 960},
		{"code 1 two frames", []byte{0x69, 0x00}, 1920},
		{"code 2 two frames", []byte{0x6a, 0x00}, 1920},
		{"code 3 one frame", []byte{0x6b, 0x01}, 960},
		{"code 3 five frames (DeepSeek)", []byte{0x6b, 0x05}, 4800},
		{"code 3 five frames VBR", []byte{0x6b, 0x85}, 4800},
		{"celt 2.5ms", []byte{0x80}, 120},
		{"celt 5ms", []byte{0x88}, 240},
		{"celt 10ms", []byte{0x90}, 480},
		{"celt 20ms", []byte{0x98}, 960},
	}
	for _, tc := range tests {
		if got := opusPacketSamples(tc.packet); got != tc.want {
			t.Errorf("%s: samples=%d want %d", tc.name, got, tc.want)
		}
	}
	if got := opusPacketSamples(nil); got != 0 {
		t.Errorf("empty packet samples=%d want 0", got)
	}
}
