package app

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"

	"translate-plugin/internal/uidaemon"
)

func TestAttachBlurredBackgrounds(t *testing.T) {
	frame := image.NewRGBA(image.Rect(0, 0, 200, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 200; x++ {
			frame.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 120, A: 255})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, frame); err != nil {
		t.Fatal(err)
	}

	blocks := []uidaemon.TranslatedBlock{
		{Text: "hello", Translation: "你好", X: 20, Y: 20, W: 80, H: 30},
		{Text: "skip", Translation: "   ", X: 0, Y: 0, W: 10, H: 10},
		{Text: "edge", Translation: "边缘", X: -40, Y: -30, W: 60, H: 50},
	}
	attachBlurredBackgrounds(encoded.Bytes(), blocks)

	if !strings.HasPrefix(blocks[0].BackgroundImage, "data:image/jpeg;base64,") {
		t.Fatalf("missing background for translated block: %q", blocks[0].BackgroundImage)
	}
	if blocks[0].TextColor != "#FFFFFF" && blocks[0].TextColor != "#101010" {
		t.Fatalf("unexpected auto text colour %q", blocks[0].TextColor)
	}
	if blocks[1].BackgroundImage != "" {
		t.Fatalf("blank translation should not get a background")
	}
	if blocks[2].BackgroundImage == "" {
		t.Fatalf("partially off-screen block should be clamped, not skipped")
	}

	payload := strings.TrimPrefix(blocks[0].BackgroundImage, "data:image/jpeg;base64,")
	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := jpeg.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if bounds := decoded.Bounds(); bounds.Dx() < 2 || bounds.Dy() < 2 {
		t.Fatalf("blurred patch too small: %v", bounds)
	}
}
