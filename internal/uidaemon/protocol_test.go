package uidaemon

import (
	"path/filepath"
	"testing"
)

func TestEndpointRoundTrip(t *testing.T) {
	dir := t.TempDir()
	endpoint := Endpoint{
		URL:   "http://127.0.0.1:12345",
		Token: "abc",
		PID:   42,
	}
	if err := WriteEndpoint(dir, endpoint); err != nil {
		t.Fatal(err)
	}
	got, err := ReadEndpoint(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.URL != endpoint.URL || got.Token != endpoint.Token || got.PID != endpoint.PID {
		t.Fatalf("unexpected endpoint: %+v", got)
	}
	if EndpointPath(dir) != filepath.Join(dir, "translate-ui-endpoint.json") {
		t.Fatal("unexpected endpoint path")
	}
	RemoveEndpoint(dir)
}
