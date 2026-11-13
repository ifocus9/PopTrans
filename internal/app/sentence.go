package app

import (
	"math"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"translate-plugin/internal/backend"
)

// OCR line boxes describe visual lines, not sentences. Translating each visual
// line separately loses the context of a sentence wrapped across lines (very
// common in documents), so consecutive wrapped lines are grouped before they go
// to the model. The merged block keeps the original line height for font sizing
// and spans every source line it covers.
var listMarkerPattern = regexp.MustCompile(
	`^(?:[-*•·]\s+|\d{1,3}[.)、]\s+|[一二三四五六七八九十]{1,3}[、.）)]\s*|[①-⑩]\s*)`,
)

type pendingOCRGroup struct {
	line         backend.OCRLine
	lastText     string
	lastBottom   float64
	lastHeight   float64
	sourceHeight float64
	merged       bool
}

func groupOCRLines(lines []backend.OCRLine) []backend.OCRLine {
	if len(lines) < 2 {
		return lines
	}

	heights := make([]float64, 0, len(lines))
	for _, line := range lines {
		if height := ocrLineHeight(line); height > 0 {
			heights = append(heights, height)
		}
	}
	medianHeight := medianFloat(heights)

	groups := make([]backend.OCRLine, 0, len(lines))
	var current *pendingOCRGroup
	for _, line := range lines {
		if strings.TrimSpace(line.Text) == "" {
			continue
		}
		if current == nil {
			current = newPendingOCRGroup(line)
			continue
		}
		if shouldJoinOCRLines(current, line, medianHeight) {
			mergeOCRGroup(current, line)
			continue
		}
		groups = append(groups, finalizeOCRGroup(current))
		current = newPendingOCRGroup(line)
	}
	if current != nil {
		groups = append(groups, finalizeOCRGroup(current))
	}
	return groups
}

func newPendingOCRGroup(line backend.OCRLine) *pendingOCRGroup {
	height := ocrLineHeight(line)
	return &pendingOCRGroup{
		line:         line,
		lastText:     line.Text,
		lastBottom:   line.Y + line.H,
		lastHeight:   height,
		sourceHeight: height,
	}
}

func shouldJoinOCRLines(current *pendingOCRGroup, next backend.OCRLine, medianHeight float64) bool {
	if strings.TrimSpace(next.Text) == "" {
		return false
	}
	// Rotated text and code/table rows are laid out individually; merging them
	// would produce a box that no longer matches the source.
	if math.Abs(next.Angle) > 3 || math.Abs(current.line.Angle) > 3 {
		return false
	}
	if isCodeLikeOCRLine(current.lastText) || isCodeLikeOCRLine(next.Text) {
		return false
	}
	if isStandaloneMathLine(current.lastText) || isStandaloneMathLine(next.Text) {
		return false
	}
	if listMarkerPattern.MatchString(strings.TrimSpace(next.Text)) {
		return false
	}
	if next.Y < current.lastBottom-1.5 {
		// Out-of-order or overlapping boxes: never guess.
		return false
	}

	gap := next.Y - current.lastBottom
	maxGap := math.Max(medianHeight*1.2, current.lastHeight*0.9)
	if gap > maxGap {
		return false
	}

	if current.lastHeight > 0 && next.H > 0 {
		ratio := next.H / current.lastHeight
		if ratio < 0.72 || ratio > 1.35 {
			return false
		}
	}

	// A new indented block or a second column is not a wrapped continuation.
	indentLimit := math.Max(medianHeight*3, 24)
	if next.X > current.line.X+indentLimit {
		return false
	}
	if next.X >= current.line.X+current.line.W-1 &&
		next.Y < current.line.Y+current.line.H {
		return false
	}

	// A visual line ending with sentence punctuation is complete. The next line
	// starts a new sentence, even when the paragraph continues.
	return !endsSentence(current.lastText)
}

func mergeOCRGroup(current *pendingOCRGroup, next backend.OCRLine) {
	current.line.Text = joinWrappedOCRText(current.line.Text, next.Text)
	left := math.Min(current.line.X, next.X)
	top := math.Min(current.line.Y, next.Y)
	right := math.Max(current.line.X+current.line.W, next.X+next.W)
	bottom := math.Max(current.line.Y+current.line.H, next.Y+next.H)
	current.line.X = left
	current.line.Y = top
	current.line.W = right - left
	current.line.H = bottom - top
	if next.Score > 0 && (current.line.Score <= 0 || next.Score < current.line.Score) {
		current.line.Score = next.Score
	}
	if height := ocrLineHeight(next); height > current.sourceHeight {
		current.sourceHeight = height
	}
	current.lastText = next.Text
	current.lastBottom = next.Y + next.H
	current.lastHeight = ocrLineHeight(next)
	current.merged = true
}

func finalizeOCRGroup(current *pendingOCRGroup) backend.OCRLine {
	line := current.line
	if !current.merged {
		return line
	}
	// LineWidth stays zero so the renderer uses the merged x/y/w/h box. The
	// single-line height is kept in LineHeight (and therefore SourceHeight) so
	// the font still matches the source text instead of the whole paragraph.
	line.Angle = 0
	line.LineWidth = 0
	line.LineHeight = current.sourceHeight
	line.CX = 0
	line.CY = line.Y + current.sourceHeight/2
	return line
}

func ocrLineHeight(line backend.OCRLine) float64 {
	if line.LineHeight > 0 {
		return line.LineHeight
	}
	return line.H
}

func medianFloat(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j] < sorted[j-1]; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}
	middle := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[middle]
	}
	return (sorted[middle-1] + sorted[middle]) / 2
}

func joinWrappedOCRText(left, right string) string {
	left = strings.TrimRight(left, " \t")
	right = strings.TrimLeft(right, " \t")
	if left == "" {
		return right
	}
	if right == "" {
		return left
	}
	last, _ := utf8.DecodeLastRuneInString(left)
	first, _ := utf8.DecodeRuneInString(right)
	if last == '-' || last == '‐' {
		if unicode.IsLetter(lastBeforeHyphen(left)) && unicode.IsLower(first) {
			return strings.TrimSuffix(strings.TrimSuffix(left, "‐"), "-") + right
		}
	}
	if noSpaceBetween(last, first) {
		return left + right
	}
	return left + " " + right
}

func lastBeforeHyphen(text string) rune {
	trimmed := strings.TrimSuffix(strings.TrimSuffix(text, "‐"), "-")
	value, _ := utf8.DecodeLastRuneInString(trimmed)
	return value
}

func noSpaceBetween(left, right rune) bool {
	return isCJKOrNoSpaceScript(left) || isCJKOrNoSpaceScript(right)
}

func isCJKOrNoSpaceScript(value rune) bool {
	switch {
	case value >= 0x2E80 && value <= 0x9FFF, // CJK radicals, kana, Han
		value >= 0xF900 && value <= 0xFAFF, // CJK compatibility ideographs
		value >= 0xAC00 && value <= 0xD7AF, // Hangul syllables
		value >= 0x1100 && value <= 0x11FF, // Hangul jamo
		value >= 0x0E00 && value <= 0x0E7F, // Thai
		value >= 0x0E80 && value <= 0x0EFF, // Lao
		value >= 0x1000 && value <= 0x109F, // Myanmar
		value >= 0x1780 && value <= 0x17FF: // Khmer
		return true
	}
	return false
}

func endsSentence(text string) bool {
	trimmed := strings.TrimRightFunc(text, unicode.IsSpace)
	trimmed = strings.TrimRight(trimmed, `"'”’」』】）)]}`)
	if trimmed == "" {
		return false
	}
	last, _ := utf8.DecodeLastRuneInString(trimmed)
	return strings.ContainsRune(".!?。！？…؟।॥", last)
}

func isCodeLikeOCRLine(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	if strings.ContainsAny(trimmed, "{}") {
		return true
	}
	if strings.HasPrefix(trimmed, "```") ||
		strings.HasPrefix(trimmed, "#include") ||
		strings.HasPrefix(trimmed, "//") ||
		strings.HasPrefix(trimmed, "/*") ||
		strings.HasPrefix(trimmed, "def ") ||
		strings.HasPrefix(trimmed, "class ") ||
		strings.HasPrefix(trimmed, "func ") ||
		strings.HasPrefix(trimmed, "import ") ||
		strings.HasPrefix(trimmed, "from ") && strings.Contains(trimmed, " import ") ||
		strings.HasPrefix(trimmed, "<") && strings.Contains(trimmed, ">") {
		return true
	}
	if strings.Contains(trimmed, "=>") || strings.Contains(trimmed, "::") {
		return true
	}
	if strings.Count(trimmed, "|") >= 2 {
		return true
	}
	// Tabular data often relies on multiple spaces for alignment.
	return strings.Contains(trimmed, "  ") && !containsCJK(trimmed)
}

func isStandaloneMathLine(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	if strings.Contains(trimmed, "$$") ||
		strings.Contains(trimmed, `\(`) ||
		strings.Contains(trimmed, `\[`) {
		return true
	}
	const mathSymbols = "=∫∑√±≈≠≤≥∞∂∇→←↔×÷·^_"
	count := 0
	for _, value := range trimmed {
		if strings.ContainsRune(mathSymbols, value) {
			count++
		}
	}
	runes := utf8.RuneCountInString(trimmed)
	if count >= 2 && runes <= 120 {
		return true
	}
	return count >= 1 && runes <= 40
}

func containsCJK(text string) bool {
	for _, value := range text {
		if isCJKOrNoSpaceScript(value) {
			return true
		}
	}
	return false
}
