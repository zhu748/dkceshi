package client

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/gif"
	"io"
	"mime"
	"mime/multipart"
	"testing"
)

func TestAnimatedGIFPreservesEveryByteAndFrameThroughMultipart(t *testing.T) {
	palette := color.Palette{color.Black, color.White}
	first := image.NewPaletted(image.Rect(0, 0, 2, 2), palette)
	second := image.NewPaletted(image.Rect(0, 0, 2, 2), palette)
	second.SetColorIndex(0, 0, 1)
	var input bytes.Buffer
	if err := gif.EncodeAll(&input, &gif.GIF{Image: []*image.Paletted{first, second}, Delay: []int{5, 12}, LoopCount: 0}); err != nil {
		t.Fatal(err)
	}
	for _, format := range []string{"webp", "jpg"} {
		ratio := 0.8
		req := UploadFileRequest{Filename: "animation.gif", ContentType: "image/gif", Data: input.Bytes(), PhotoCompressRatio: &ratio, PictureCompressFormat: format}
		prepared, err := prepareUploadImage(context.Background(), req)
		if err != nil || !bytes.Equal(prepared.Data, req.Data) || prepared.Filename != req.Filename {
			t.Fatalf("GIF changed under %s compression: %v", format, err)
		}
		body, contentType, err := buildUploadMultipartBody(prepared.Filename, prepared.ContentType, prepared.Data)
		if err != nil {
			t.Fatal(err)
		}
		_, params, err := mime.ParseMediaType(contentType)
		if err != nil {
			t.Fatal(err)
		}
		reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
		part, err := reader.NextPart()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(part)
		if closeErr := part.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
		if err != nil || !bytes.Equal(data, req.Data) || part.Header.Get("Content-Type") != "application/octet-stream" {
			t.Fatalf("multipart changed GIF: %v", err)
		}
		decoded, err := gif.DecodeAll(bytes.NewReader(data))
		if err != nil || len(decoded.Image) != 2 || decoded.Delay[1] != 12 || decoded.LoopCount != 0 {
			t.Fatalf("animation lost frames/timing: %v", err)
		}
	}
}
