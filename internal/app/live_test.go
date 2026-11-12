package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"translate-plugin/internal/backend"
	"translate-plugin/internal/config"
	win "translate-plugin/internal/platform/windows"
)

func TestTranslateLiveFrameUsesSplitBlocksEndpoint(t *testing.T) {
	var received struct {
		Lines       []backend.OCRLine `json:"lines"`
		SourceText  string            `json:"source_text"`
		ImageWidth  int               `json:"image_width"`
		ImageHeight int               `json:"image_height"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/translate_blocks" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(backend.OCRTranslateBlocksResult{
			SourceText:  "Hello\nWorld",
			Translation: "你好\n世界",
			Blocks: []backend.OCRBlock{
				{Text: "Hello", Translation: "你好", X: 1, Y: 2, W: 30, H: 12},
				{Text: "World", Translation: "世界", X: 1, Y: 20, W: 30, H: 12},
			},
			ImageWidth:  640,
			ImageHeight: 480,
		})
	}))
	defer server.Close()

	app := &App{client: backend.NewClient(server.URL), cfg: config.Config{}}
	ocr := backend.OCRBlocksResult{
		Text:       "Hello\nWorld",
		LineBoxes:  []backend.OCRLine{{Text: "Hello", X: 1, Y: 2, W: 30, H: 12}, {Text: "World", X: 1, Y: 20, W: 30, H: 12}},
		ImageWidth: 640, ImageHeight: 480,
	}

	result, err := app.translateOCRResult(t.Context(), ocr)
	if err != nil {
		t.Fatalf("translateOCRResult: %v", err)
	}
	if len(result.Blocks) != 2 || result.Translation != "你好\n世界" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(received.Lines) != 2 || received.SourceText != "Hello\nWorld" {
		t.Fatalf("unexpected request: %+v", received)
	}
	if received.ImageWidth != 640 || received.ImageHeight != 480 {
		t.Fatalf("image size not forwarded: %+v", received)
	}
}

func TestTranslateLiveFrameFallsBackToWholeText(t *testing.T) {
	chatCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/translate_blocks":
			http.NotFound(w, r)
		case "/v1/chat/completions":
			chatCalls++
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{
					"message": map[string]string{"content": "整段译文"},
				}},
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	app := &App{client: backend.NewClient(server.URL), cfg: config.Config{}}
	result, err := app.translateOCRResult(t.Context(), backend.OCRBlocksResult{
		Text:      "Hello\nWorld",
		LineBoxes: []backend.OCRLine{{Text: "Hello"}},
	})
	if err != nil {
		t.Fatalf("translateOCRResult fallback: %v", err)
	}
	if chatCalls != 1 || result.Translation != "整段译文" || result.SourceText != "Hello\nWorld" {
		t.Fatalf("unexpected fallback result: %+v chats=%d", result, chatCalls)
	}
}

func TestTranslateLiveFrameBlankRegionIsQuiet(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Error(w, "should not be called", http.StatusInternalServerError)
	}))
	defer server.Close()

	app := &App{client: backend.NewClient(server.URL), cfg: config.Config{}}
	result, err := app.translateOCRResult(t.Context(), backend.OCRBlocksResult{})
	if err != nil {
		t.Fatalf("blank region returned error: %v", err)
	}
	if result.SourceText != "" || result.Translation != "" || len(result.Blocks) != 0 {
		t.Fatalf("blank region returned content: %+v", result)
	}
	if calls != 0 {
		t.Fatalf("blank region hit the backend %d times", calls)
	}
}

func TestLivePayloadReportsProgress(t *testing.T) {
	app := &App{cfg: config.Config{}}
	anchor := win.Rect{Left: 10, Top: 20, Right: 310, Bottom: 220}
	window := win.Rect{Left: 8, Top: 0, Right: 312, Bottom: 260}
	result := backend.OCRTranslateBlocksResult{
		SourceText:  "Hello",
		Translation: "你好",
		Blocks:      []backend.OCRBlock{{Text: "Hello", Translation: "你好", W: 80, H: 20}},
	}

	payload := app.livePayloadFrom(anchor, window, "top", nil, result, nil)
	if payload.Stage != "done" || payload.Progress != 100 || payload.Lines != 1 {
		t.Fatalf("success payload progress = %+v", payload)
	}

	failed := app.livePayloadFrom(anchor, window, "top", nil, backend.OCRTranslateBlocksResult{}, errors.New("boom"))
	if failed.Stage != "error" || failed.Progress != 0 {
		t.Fatalf("error payload progress = %+v", failed)
	}
}
