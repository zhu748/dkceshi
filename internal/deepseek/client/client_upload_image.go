package client

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"path/filepath"
	"strings"

	"github.com/HugoSmits86/nativewebp"
)

// The captures contain WebP photos, but do not reveal the App's resize/quality
// settings. Encode decoded pixels losslessly; preserve unsupported/animated
// formats and large images instead of silently discarding frames or allocating
// unbounded decoded buffers. Documents are never transcoded.
func prepareUploadImage(ctx context.Context, req UploadFileRequest) (UploadFileRequest, error) {
	if req.PreserveImage {
		return req, nil
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(req.Data))
	if err != nil || (format != "jpeg" && format != "png") {
		return req, nil
	}
	const maxPixels = 20_000_000
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 16383 || cfg.Height > 16383 || int64(cfg.Width)*int64(cfg.Height) > maxPixels {
		return req, nil
	}
	// APNG has animation chunks; converting it as a static PNG would lose frames.
	if format == "png" && bytes.Contains(req.Data, []byte("acTL")) {
		return req, nil
	}
	if err := ctx.Err(); err != nil {
		return req, err
	}
	img, _, err := image.Decode(bytes.NewReader(req.Data))
	if err != nil {
		return req, err
	}
	if format == "jpeg" {
		img = orientUploadImage(img, jpegOrientation(req.Data))
	}
	var output bytes.Buffer
	if err := nativewebp.Encode(&output, img, &nativewebp.Options{CompressionLevel: nativewebp.BestSpeed}); err != nil {
		return req, err
	}
	if err := ctx.Err(); err != nil {
		return req, err
	}
	filename := strings.TrimSpace(req.Filename)
	if filename == "" {
		filename = "image"
	}
	req.Filename = strings.TrimSuffix(filename, filepath.Ext(filename)) + ".webp"
	req.ContentType = "image/webp"
	req.Data = output.Bytes()
	return req, nil
}

// jpegOrientation reads only the bounded EXIF IFD0 orientation field.
func jpegOrientation(data []byte) int {
	if len(data) < 2 || data[0] != 0xff || data[1] != 0xd8 {
		return 1
	}
	for offset := 2; offset+4 <= len(data); {
		if data[offset] != 0xff {
			break
		}
		marker := data[offset+1]
		if marker == 0xda || marker == 0xd9 {
			break
		}
		if marker == 0xff {
			offset++
			continue
		}
		length := int(binary.BigEndian.Uint16(data[offset+2 : offset+4]))
		if length < 2 || length > len(data)-offset-2 {
			break
		}
		segment := data[offset+4 : offset+2+length]
		if marker == 0xe1 && bytes.HasPrefix(segment, []byte("Exif\x00\x00")) {
			return tiffOrientation(segment[6:])
		}
		offset += 2 + length
	}
	return 1
}

func tiffOrientation(data []byte) int {
	if len(data) < 8 {
		return 1
	}
	var order binary.ByteOrder
	switch string(data[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 1
	}
	if order.Uint16(data[2:4]) != 42 {
		return 1
	}
	offset := uint64(order.Uint32(data[4:8]))
	if offset+2 > uint64(len(data)) {
		return 1
	}
	count := uint64(order.Uint16(data[offset : offset+2]))
	for i := uint64(0); i < count; i++ {
		start := offset + 2 + i*12
		if start+12 > uint64(len(data)) {
			return 1
		}
		entry := data[start : start+12]
		if order.Uint16(entry[:2]) == 0x112 && order.Uint16(entry[2:4]) == 3 && order.Uint32(entry[4:8]) == 1 {
			value := int(order.Uint16(entry[8:10]))
			if value >= 1 && value <= 8 {
				return value
			}
			return 1
		}
	}
	return 1
}

func orientUploadImage(src image.Image, orientation int) image.Image {
	if orientation <= 1 || orientation > 8 {
		return src
	}
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	outW, outH := w, h
	if orientation >= 5 {
		outW, outH = h, w
	}
	dst := image.NewNRGBA(image.Rect(0, 0, outW, outH))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx, dy := x, y
			switch orientation {
			case 2:
				dx = w - 1 - x
			case 3:
				dx, dy = w-1-x, h-1-y
			case 4:
				dy = h - 1 - y
			case 5:
				dx, dy = y, x
			case 6:
				dx, dy = h-1-y, x
			case 7:
				dx, dy = h-1-y, w-1-x
			case 8:
				dx, dy = y, w-1-x
			}
			dst.Set(dx, dy, src.At(bounds.Min.X+x, bounds.Min.Y+y))
		}
	}
	return dst
}
