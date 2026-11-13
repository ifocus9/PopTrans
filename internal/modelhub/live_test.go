package modelhub

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestLiveSearch(t *testing.T) {
	if os.Getenv("POPTRANS_LIVE_TEST") != "1" {
		t.Skip("set POPTRANS_LIVE_TEST=1 to hit the network")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	results, err := Search(ctx, SourceModelScope, "Hy-MT")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("no results")
	}
	t.Logf("results: %d first=%s size=%d params=%s", len(results), results[0].ID, results[0].SizeBytes, results[0].Parameters)
	files, err := Files(ctx, SourceModelScope, "Tencent-Hunyuan/Hy-MT2-1.8B-GGUF")
	if err != nil {
		t.Fatalf("files: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("no files")
	}
	t.Logf("files: %d first=%s (%d bytes)", len(files), files[0].Name, files[0].Size)
}

func TestLiveSearchFiltersNonGGUF(t *testing.T) {
	if os.Getenv("POPTRANS_LIVE_TEST") != "1" {
		t.Skip("set POPTRANS_LIVE_TEST=1 to hit the network")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	results, err := Search(ctx, SourceModelScope, "Qwen2.5-0.5B")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("no GGUF results")
	}
	for index, result := range results {
		if index >= 5 {
			break
		}
		if result.SizeBytes <= 0 || result.Parameters == "" {
			t.Logf("result %d missing metadata: %+v", index, result)
		}
		t.Logf("result %d: %s size=%d params=%s", index, result.ID, result.SizeBytes, result.Parameters)
		files, err := Files(ctx, SourceModelScope, result.ID)
		if err != nil {
			t.Fatalf("files %s: %v", result.ID, err)
		}
		if len(files) == 0 {
			t.Fatalf("non-GGUF result leaked through: %s", result.ID)
		}
	}
}

func TestLiveDownloadStart(t *testing.T) {
	if os.Getenv("POPTRANS_LIVE_TEST") != "1" {
		t.Skip("set POPTRANS_LIVE_TEST=1 to hit the network")
	}
	dest := t.TempDir() + "/probe.gguf"
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	var last Progress
	err := Download(ctx, SourceModelScope, "Tencent-Hunyuan/Hy-MT2-1.8B-GGUF",
		"Hy-MT2-1.8B-Q4_K_M.gguf", dest, func(p Progress) { last = p })
	if err == nil {
		t.Fatalf("expected the short timeout to cancel the download")
	}
	if last.Downloaded == 0 {
		t.Fatalf("no bytes downloaded before cancel: %v", err)
	}
	if _, statErr := os.Stat(dest + ".part"); statErr != nil {
		t.Fatalf("missing .part file: %v", statErr)
	}
	t.Logf("canceled after %d bytes: %v", last.Downloaded, err)
}

func TestLiveSearchResultDownload(t *testing.T) {
	if os.Getenv("POPTRANS_LIVE_TEST") != "1" {
		t.Skip("set POPTRANS_LIVE_TEST=1 to hit the network")
	}
	dest := t.TempDir() + "/qwen.gguf"
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	var last Progress
	err := Download(ctx, SourceModelScope, "Qwen/Qwen2.5-0.5B-Instruct-GGUF",
		"qwen2.5-0.5b-instruct-q2_k.gguf", dest, func(p Progress) { last = p })
	if err == nil {
		t.Fatalf("expected the short timeout to cancel the download")
	}
	if last.Downloaded == 0 {
		t.Fatalf("no bytes downloaded: %v", err)
	}
	t.Logf("downloaded %d bytes before cancel, total=%d, err=%v", last.Downloaded, last.Total, err)
}
