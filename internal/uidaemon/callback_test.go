package uidaemon

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestCallbackServerHotkeyHoldEvents(t *testing.T) {
	events := make(chan int, 4)
	server := NewCallbackServer(func(event int) {
		events <- event
	})
	url, token, err := server.Start()
	if err != nil {
		t.Fatalf("start callback server: %v", err)
	}
	defer server.Stop()

	client := NewNotifyClient(url, token)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := client.HotkeyHold(ctx, true); err != nil {
		t.Fatalf("hotkey pause: %v", err)
	}
	select {
	case event := <-events:
		if event != EventHotkeyPause {
			t.Fatalf("event = %d, want %d", event, EventHotkeyPause)
		}
	case <-time.After(time.Second):
		t.Fatal("hotkey pause event not delivered")
	}

	if err := client.HotkeyHold(ctx, false); err != nil {
		t.Fatalf("hotkey resume: %v", err)
	}
	select {
	case event := <-events:
		if event != EventHotkeyResume {
			t.Fatalf("event = %d, want %d", event, EventHotkeyResume)
		}
	case <-time.After(time.Second):
		t.Fatal("hotkey resume event not delivered")
	}
}

func TestCallbackServerRejectsInvalidHotkeyHoldPayload(t *testing.T) {
	server := NewCallbackServer(nil)
	url, token, err := server.Start()
	if err != nil {
		t.Fatalf("start callback server: %v", err)
	}
	defer server.Stop()

	req, err := http.NewRequest(http.MethodPost, url+EndpointNotifyHotkeyHold, strings.NewReader("{bad json"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(TokenHeader(), token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

