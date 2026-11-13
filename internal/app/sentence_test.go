package app

import (
	"strings"
	"testing"

	"translate-plugin/internal/backend"
	"translate-plugin/internal/uidaemon"
)

func TestGroupOCRLinesJoinsWrappedEnglishSentence(t *testing.T) {
	lines := []backend.OCRLine{
		{Text: "This sentence wraps onto", X: 20, Y: 10, W: 300, H: 20, LineWidth: 300, LineHeight: 20, CY: 20},
		{Text: "a second visual line.", X: 20, Y: 34, W: 240, H: 20, LineWidth: 240, LineHeight: 20, CY: 44},
	}
	groups := groupOCRLines(lines)
	if len(groups) != 1 {
		t.Fatalf("groups = %d, want 1: %+v", len(groups), groups)
	}
	group := groups[0]
	if group.Text != "This sentence wraps onto a second visual line." {
		t.Fatalf("text = %q", group.Text)
	}
	if group.LineWidth != 0 || group.LineHeight != 20 {
		t.Fatalf("geometry = %+v", group)
	}
	if group.H < 40 || group.W != 300 {
		t.Fatalf("merged box = %+v", group)
	}
	if group.CY != group.Y+10 {
		t.Fatalf("CY = %v, want %v", group.CY, group.Y+10)
	}
}

func TestGroupOCRLinesJoinsChineseWithoutSpaces(t *testing.T) {
	lines := []backend.OCRLine{
		{Text: "这是一句被换行打断的话，", X: 10, Y: 10, W: 400, H: 24, LineWidth: 400, LineHeight: 24, CY: 22},
		{Text: "合并后翻译才准确。", X: 10, Y: 38, W: 300, H: 24, LineWidth: 300, LineHeight: 24, CY: 50},
	}
	groups := groupOCRLines(lines)
	if len(groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(groups))
	}
	if groups[0].Text != "这是一句被换行打断的话，合并后翻译才准确。" {
		t.Fatalf("text = %q", groups[0].Text)
	}
}

func TestGroupOCRLinesKeepsCompleteSentencesSeparate(t *testing.T) {
	lines := []backend.OCRLine{
		{Text: "First sentence.", X: 10, Y: 10, W: 200, H: 20, LineWidth: 200, LineHeight: 20},
		{Text: "Second sentence.", X: 10, Y: 34, W: 200, H: 20, LineWidth: 200, LineHeight: 20},
	}
	groups := groupOCRLines(lines)
	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(groups))
	}
}

func TestGroupOCRLinesHonoursParagraphGap(t *testing.T) {
	lines := []backend.OCRLine{
		{Text: "A line without terminal punctuation", X: 10, Y: 10, W: 300, H: 20, LineWidth: 300, LineHeight: 20},
		{Text: "Looks adjacent but is a new paragraph", X: 10, Y: 80, W: 300, H: 20, LineWidth: 300, LineHeight: 20},
	}
	if groups := groupOCRLines(lines); len(groups) != 2 {
		t.Fatalf("groups = %d, want 2: %+v", len(groups), groups)
	}
}

func TestGroupOCRLinesKeepsListsAndMathSeparate(t *testing.T) {
	cases := []struct {
		name  string
		lines []backend.OCRLine
	}{
		{
			name: "list",
			lines: []backend.OCRLine{
				{Text: "Intro without a period", X: 10, Y: 10, W: 300, H: 20, LineWidth: 300, LineHeight: 20},
				{Text: "- first item", X: 10, Y: 34, W: 200, H: 20, LineWidth: 200, LineHeight: 20},
			},
		},
		{
			name: "formula",
			lines: []backend.OCRLine{
				{Text: "The relation is", X: 10, Y: 10, W: 200, H: 20, LineWidth: 200, LineHeight: 20},
				{Text: "E = mc^2", X: 10, Y: 34, W: 160, H: 20, LineWidth: 160, LineHeight: 20},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if groups := groupOCRLines(tc.lines); len(groups) != 2 {
				t.Fatalf("groups = %d, want 2: %+v", len(groups), groups)
			}
		})
	}
}

func TestGroupOCRLinesJoinsThreeLinesAndKeepsSingleLineHeight(t *testing.T) {
	lines := []backend.OCRLine{
		{Text: "A single sentence", X: 10, Y: 10, W: 280, H: 20, LineWidth: 280, LineHeight: 20},
		{Text: "that continues", X: 10, Y: 34, W: 180, H: 20, LineWidth: 180, LineHeight: 20},
		{Text: "over three lines.", X: 10, Y: 58, W: 200, H: 20, LineWidth: 200, LineHeight: 20},
	}
	groups := groupOCRLines(lines)
	if len(groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(groups))
	}
	group := groups[0]
	if !strings.Contains(group.Text, "over three lines.") {
		t.Fatalf("text = %q", group.Text)
	}
	if group.LineHeight != 20 || group.H != 68 {
		t.Fatalf("line height/box = %+v", group)
	}
}

func TestClipBlockOverlapsSkipsSentenceGroups(t *testing.T) {
	blocks := []uidaemon.TranslatedBlock{
		{Y: 10, H: 60, SourceHeight: 20, LineHeight: 20, CY: 20, X: 0, W: 100},
		{Y: 80, H: 20, SourceHeight: 20, LineHeight: 20, CY: 90, X: 0, W: 100},
	}
	clipBlockOverlaps(blocks)
	if blocks[0].H != 60 {
		t.Fatalf("sentence group was clipped: %+v", blocks[0])
	}
	if blocks[1].H != 20 {
		t.Fatalf("following line was clipped: %+v", blocks[1])
	}
}
