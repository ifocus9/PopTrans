package uidaemon

import (
	"context"
	"testing"
	"time"
)

func TestCallbackServerRefreshEvents(t *testing.T) {
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

	if err := client.OverlayRefresh(ctx); err != nil {
		t.Fatalf("overlay refresh: %v", err)
	}
	select {
	case event := <-events:
		if event != EventOverlayRefresh {
			t.Fatalf("event = %d, want %d", event, EventOverlayRefresh)
		}
	case <-time.After(time.Second):
		t.Fatal("overlay refresh event not delivered")
	}

	if err := client.LiveControl(ctx, "refresh"); err != nil {
		t.Fatalf("live refresh: %v", err)
	}
	select {
	case event := <-events:
		if event != EventLiveRefresh {
			t.Fatalf("event = %d, want %d", event, EventLiveRefresh)
		}
	case <-time.After(time.Second):
		t.Fatal("live refresh event not delivered")
	}
}
