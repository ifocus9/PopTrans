package app

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/draw"
	"image/jpeg"
	"math"
	"strings"

	"translate-plugin/internal/uidaemon"
)

const (
	blurBackgroundPad    = 5
	// A strong blur removes the source glyph shapes from the chip background, so
	// only the translated text is readable over the original line.
	blurBackgroundRadius = 14
)

// attachBlurredBackgrounds crops the captured screen behind every translated
// line, blurs it and stores it as a small JPEG data URL. The overlay renders
// these as frosted-glass chips, so text stays readable while everything outside
// the text remains the real, sharp desktop.
func attachBlurredBackgrounds(pngBytes []byte, blocks []uidaemon.TranslatedBlock) {
	if len(pngBytes) == 0 || len(blocks) == 0 {
		return
	}
	frame, _, err := image.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		return
	}
	bounds := frame.Bounds()
	for index := range blocks {
		block := &blocks[index]
		if strings.TrimSpace(block.Translation) == "" {
			continue
		}
		x0 := int(math.Floor(block.X)) - blurBackgroundPad
		y0 := int(math.Floor(block.Y)) - blurBackgroundPad
		x1 := int(math.Ceil(block.X+block.W)) + blurBackgroundPad
		y1 := int(math.Ceil(block.Y+block.H)) + blurBackgroundPad
		rect := image.Rect(x0, y0, x1, y1).Intersect(bounds)
		if rect.Empty() || rect.Dx() < 2 || rect.Dy() < 2 {
			continue
		}
		patch := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
		draw.Draw(patch, patch.Bounds(), frame, rect.Min, draw.Src)
		boxBlur(patch, blurBackgroundRadius)
		// Pick the text tone from the blurred background luminance. The UI also
		// adds a light/dark scrim so abrupt background changes stay readable.
		if averageLuminance(patch) >= 0.58 {
			block.TextColor = "#101010"
		} else {
			block.TextColor = "#FFFFFF"
		}

		var buffer bytes.Buffer
		if err := jpeg.Encode(&buffer, patch, &jpeg.Options{Quality: 80}); err != nil {
			continue
		}
		block.BackgroundImage = "data:image/jpeg;base64," +
			base64.StdEncoding.EncodeToString(buffer.Bytes())
	}
}

// boxBlur is a separable box blur (horizontal then vertical) with clamped
// edges. It runs in O(width*height) and is only used on small text crops.
func boxBlur(img *image.RGBA, radius int) {
	if radius < 1 {
		return
	}
	width, height := img.Bounds().Dx(), img.Bounds().Dy()
	if width < 2 || height < 2 {
		return
	}
	temp := image.NewRGBA(image.Rect(0, 0, width, height))
	window := int64(radius*2 + 1)

	for y := 0; y < height; y++ {
		row := y * img.Stride
		for channel := 0; channel < 4; channel++ {
			var sum int64
			for k := -radius; k <= radius; k++ {
				sum += int64(img.Pix[row+clampInt(k, 0, width-1)*4+channel])
			}
			for x := 0; x < width; x++ {
				temp.Pix[y*temp.Stride+x*4+channel] = uint8(sum / window)
				remove := clampInt(x-radius, 0, width-1)
				add := clampInt(x+radius+1, 0, width-1)
				sum += int64(img.Pix[row+add*4+channel]) - int64(img.Pix[row+remove*4+channel])
			}
		}
	}

	for x := 0; x < width; x++ {
		for channel := 0; channel < 4; channel++ {
			var sum int64
			for k := -radius; k <= radius; k++ {
				sum += int64(temp.Pix[clampInt(k, 0, height-1)*temp.Stride+x*4+channel])
			}
			for y := 0; y < height; y++ {
				img.Pix[y*img.Stride+x*4+channel] = uint8(sum / window)
				remove := clampInt(y-radius, 0, height-1)
				add := clampInt(y+radius+1, 0, height-1)
				sum += int64(temp.Pix[add*temp.Stride+x*4+channel]) -
					int64(temp.Pix[remove*temp.Stride+x*4+channel])
			}
		}
	}
}

func clampInt(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

// averageLuminance returns the perceived brightness (0..1) of the patch,
// sampling every other pixel for speed.
func averageLuminance(img *image.RGBA) float64 {
	bounds := img.Bounds()
	if bounds.Empty() {
		return 0
	}
	var sum float64
	var count int
	for y := bounds.Min.Y; y < bounds.Max.Y; y += 2 {
		for x := bounds.Min.X; x < bounds.Max.X; x += 2 {
			offset := img.PixOffset(x, y)
			r := float64(img.Pix[offset]) / 255
			g := float64(img.Pix[offset+1]) / 255
			b := float64(img.Pix[offset+2]) / 255
			sum += 0.2126*r + 0.7152*g + 0.0722*b
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}
