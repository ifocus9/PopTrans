package windows

import (
	"image"
	"image/color"
	"testing"
)

func fingerprintTestFrame() *image.RGBA {
	frame := image.NewRGBA(image.Rect(0, 0, 1040, 440))
	for y := 0; y < 440; y++ {
		for x := 0; x < 1040; x++ {
			frame.Set(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}
	return frame
}

func fillRect(frame *image.RGBA, rect image.Rectangle, value uint8) {
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			frame.Set(x, y, color.RGBA{R: value, G: value, B: value, A: 255})
		}
	}
}

func TestFingerprintIgnoresCaretBlinkAndMove(t *testing.T) {
	base := FingerprintImage(fingerprintTestFrame())

	withCaret := fingerprintTestFrame()
	fillRect(withCaret, image.Rect(100, 120, 101, 138), 0)
	if !FingerprintImage(withCaret).Similar(base) {
		t.Fatal("a one-pixel caret should not count as a content change")
	}

	movedCaret := fingerprintTestFrame()
	fillRect(movedCaret, image.Rect(108, 120, 109, 138), 0)
	if !FingerprintImage(movedCaret).Similar(base) {
		t.Fatal("moving a caret should not count as a content change")
	}
}

func TestFingerprintDetectsGlyphChange(t *testing.T) {
	base := FingerprintImage(fingerprintTestFrame())
	withGlyph := fingerprintTestFrame()
	fillRect(withGlyph, image.Rect(100, 120, 112, 136), 0)

	if FingerprintImage(withGlyph).Similar(base) {
		t.Fatal("a changed glyph should trigger a refresh")
	}
}

func TestFingerprintDetectsLargeChange(t *testing.T) {
	base := FingerprintImage(fingerprintTestFrame())
	changed := fingerprintTestFrame()
	fillRect(changed, image.Rect(0, 200, 1040, 260), 40)

	if FingerprintImage(changed).Similar(base) {
		t.Fatal("a large visual change should trigger a refresh")
	}
}

func TestFingerprintDetectsBrightnessShift(t *testing.T) {
	base := FingerprintImage(fingerprintTestFrame())
	dimmer := fingerprintTestFrame()
	for y := 0; y < 440; y++ {
		for x := 0; x < 1040; x++ {
			dimmer.Set(x, y, color.RGBA{R: 246, G: 246, B: 246, A: 255})
		}
	}

	if FingerprintImage(dimmer).Similar(base) {
		t.Fatal("a global brightness shift should trigger a refresh")
	}
}

func TestFingerprintDifferentSizesAreNotSimilar(t *testing.T) {
	first := FingerprintImage(fingerprintTestFrame())
	second := FingerprintImage(image.NewRGBA(image.Rect(0, 0, 640, 480)))
	if first.Similar(second) {
		t.Fatal("different geometry must never be treated as similar")
	}
}
