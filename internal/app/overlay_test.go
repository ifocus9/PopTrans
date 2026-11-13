package app

import (
	"testing"
	"time"

	"translate-plugin/internal/backend"
	"translate-plugin/internal/uidaemon"
)

func TestOverlayResultPayloadEmptyCaptureIsNotAnError(t *testing.T) {
	app := &App{}
	payload := app.overlayResultPayload(
		uidaemon.OverlayPayload{ImageWidth: 100, ImageHeight: 50},
		nil,
		backend.OCRTranslateBlocksResult{},
		nil,
		120*time.Millisecond,
		nil,
	)
	if payload.Error != "" {
		t.Fatalf("empty capture produced an error: %q", payload.Error)
	}
	if payload.Stage != "done" || payload.Progress != 100 || payload.Lines != 0 {
		t.Fatalf("unexpected empty payload state: %+v", payload)
	}
	if len(payload.Blocks) != 0 {
		t.Fatalf("empty capture produced blocks: %+v", payload.Blocks)
	}
}
