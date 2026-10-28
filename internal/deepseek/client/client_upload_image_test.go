package client

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"golang.org/x/image/webp"
)

func uploadTestImage() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 3; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 80), G: uint8(y * 90), B: 40, A: 200})
		}
	}
	return img
}

func TestUploadConvertsPNGToRealWebP(t *testing.T) {
	img := uploadTestImage()
	var original bytes.Buffer
	if err := png.Encode(&original, img); err != nil {
		t.Fatal(err)
	}
	req := UploadFileRequest{Filename: "photo.png", ContentType: "image/png", Data: original.Bytes()}
	got, err := prepareUploadImage(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if got.Filename != "photo.webp" || got.ContentType != "image/webp" {
		t.Fatalf("incorrect metadata: %s %s", got.Filename, got.ContentType)
	}
	decoded, err := webp.Decode(bytes.NewReader(got.Data))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Bounds() != img.Bounds() {
		t.Fatalf("dimensions changed: %v", decoded.Bounds())
	}
	for y := 0; y < 2; y++ {
		for x := 0; x < 3; x++ {
			if a, b := color.NRGBAModel.Convert(img.At(x, y)), color.NRGBAModel.Convert(decoded.At(x, y)); a != b {
				t.Fatalf("pixel changed at %d,%d: %v != %v", x, y, a, b)
			}
		}
	}
	if !bytes.Equal(req.Data, original.Bytes()) || req.Filename != "photo.png" {
		t.Fatal("input was mutated")
	}
}

func TestUploadJPEGOrientationAndPreservation(t *testing.T) {
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, uploadTestImage(), nil); err != nil {
		t.Fatal(err)
	}
	exif := []byte{'E', 'x', 'i', 'f', 0, 0, 'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0, 0x12, 1, 3, 0, 1, 0, 0, 0, 6, 0, 0, 0, 0, 0, 0, 0}
	var segment bytes.Buffer
	segment.Write([]byte{0xff, 0xe1})
	var length [2]byte
	binary.BigEndian.PutUint16(length[:], uint16(len(exif)+2))
	segment.Write(length[:])
	segment.Write(exif)
	data := append([]byte(nil), encoded.Bytes()[:2]...)
	data = append(data, segment.Bytes()...)
	data = append(data, encoded.Bytes()[2:]...)
	req := UploadFileRequest{Filename: "camera.jpg", ContentType: "image/jpeg", Data: data}
	got, err := prepareUploadImage(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := webp.DecodeConfig(bytes.NewReader(got.Data))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Width != 2 || cfg.Height != 3 {
		t.Fatalf("EXIF rotation not applied: %#v", cfg)
	}
	req.PreserveImage = true
	preserved, err := prepareUploadImage(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if preserved.Filename != req.Filename || !bytes.Equal(preserved.Data, data) {
		t.Fatal("preserve_image changed bytes")
	}
	for _, data := range [][]byte{[]byte("%PDF-1.7\nreport"), []byte("plain text"), []byte("GIF89a"), got.Data} {
		request := UploadFileRequest{Filename: "original.bin", Data: data}
		result, err := prepareUploadImage(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(result.Data, data) || result.Filename != request.Filename {
			t.Fatal("non-JPEG/PNG file was changed")
		}
	}
}

func TestUploadImageHonorsCancellation(t *testing.T) {
	var imageData bytes.Buffer
	if err := png.Encode(&imageData, uploadTestImage()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := prepareUploadImage(ctx, UploadFileRequest{Data: imageData.Bytes()}); err == nil {
		t.Fatal("expected cancellation")
	}
}
