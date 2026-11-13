package windows

import "image"

const (
	// A 128px-wide grayscale thumbnail is enough to notice a changed word or a
	// new dialog while ignoring a blinking caret or one-pixel cursor movement.
	fingerprintWidth        = 128
	fingerprintMaxHeight    = 256
	fingerprintChangedDelta = 48
	// Skip OCR/translation only when the frames are this close. A single changed
	// glyph normally covers 2-4 thumbnail cells; a caret covers at most one.
	fingerprintMaxMeanDiff     = 0.9
	fingerprintMaxChangedRatio = 0.00025
)

// FrameFingerprint is a low-resolution grayscale summary of a captured frame.
type FrameFingerprint struct {
	Width  int
	Height int
	Gray   []byte
}

// FingerprintImage downsamples an RGBA frame into a compact grayscale grid.
// The capture path already holds the pixels, so this costs a few milliseconds.
func FingerprintImage(img *image.RGBA) FrameFingerprint {
	if img == nil {
		return FrameFingerprint{}
	}
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 {
		return FrameFingerprint{}
	}
	thumbWidth := fingerprintWidth
	if width < thumbWidth {
		thumbWidth = width
	}
	thumbHeight := (thumbWidth*height + width/2) / width
	if thumbHeight < 1 {
		thumbHeight = 1
	}
	if thumbHeight > fingerprintMaxHeight {
		thumbHeight = fingerprintMaxHeight
	}

	columns := make([]int, width)
	for x := range columns {
		columns[x] = x * thumbWidth / width
	}
	rows := make([]int, height)
	for y := range rows {
		rows[y] = y * thumbHeight / height
	}

	sums := make([]uint32, thumbWidth*thumbHeight)
	counts := make([]uint32, thumbWidth*thumbHeight)
	for y := 0; y < height; y++ {
		row := img.Pix[y*img.Stride:]
		base := rows[y] * thumbWidth
		for x := 0; x < width; x++ {
			pixel := row[x*4:]
			luma := (299*uint32(pixel[0]) + 587*uint32(pixel[1]) + 114*uint32(pixel[2])) / 1000
			index := base + columns[x]
			sums[index] += luma
			counts[index]++
		}
	}

	gray := make([]byte, len(sums))
	for i := range gray {
		if counts[i] > 0 {
			gray[i] = byte(sums[i] / counts[i])
		}
	}
	return FrameFingerprint{Width: thumbWidth, Height: thumbHeight, Gray: gray}
}

// Similar reports whether two frames are close enough that re-running OCR and
// translation would produce the same overlay.
func (a FrameFingerprint) Similar(b FrameFingerprint) bool {
	if a.Width <= 0 || a.Width != b.Width || a.Height != b.Height || len(a.Gray) != len(b.Gray) {
		return false
	}
	total := len(a.Gray)
	if total == 0 {
		return false
	}
	var sum int
	changed := 0
	for i := range a.Gray {
		diff := int(a.Gray[i]) - int(b.Gray[i])
		if diff < 0 {
			diff = -diff
		}
		sum += diff
		if diff >= fingerprintChangedDelta {
			changed++
		}
	}
	mean := float64(sum) / float64(total)
	ratio := float64(changed) / float64(total)
	return mean <= fingerprintMaxMeanDiff && ratio <= fingerprintMaxChangedRatio
}
