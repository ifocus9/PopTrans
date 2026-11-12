// Package modelhub talks to ModelScope / HuggingFace to search GGUF models and
// download the selected file. Every URL (including redirect hops) is validated
// so the app never fetches localhost, private, link-local or reserved hosts.
package modelhub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	SourceModelScope  = "modelscope"
	SourceHuggingFace = "huggingface"

	modelScopeEndpoint  = "https://modelscope.cn"
	huggingFaceEndpoint = "https://huggingface.co"
	maxRedirects        = 5
)

// SearchResult is one model repository returned by a search.
type SearchResult struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Source      string `json:"source"`
	Downloads   int64  `json:"downloads,omitempty"`
	Description string `json:"description,omitempty"`
	// SizeBytes is the repository's storage size (all files/quants) when the
	// source reports it; 0 when unknown.
	SizeBytes int64 `json:"size_bytes,omitempty"`
	// Parameters is a display string such as "1.8B", parsed from the repo name.
	Parameters string `json:"parameters,omitempty"`
}

// FileInfo is one downloadable file in a repository.
type FileInfo struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// Progress reports a running download.
type Progress struct {
	Downloaded int64
	Total      int64
}

var httpClient = &http.Client{
	Timeout: 45 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return fmt.Errorf("too many redirects")
		}
		return ValidateURL(req.Context(), req.URL.String())
	},
}

// ValidateURL enforces the SSRF policy: only http(s) and never local/private
// hosts. It also resolves the hostname and rejects any non-public address.
func ValidateURL(ctx context.Context, raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("unsupported url scheme %q", parsed.Scheme)
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "" {
		return errors.New("url has no host")
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return fmt.Errorf("refusing local host %q", host)
	}
	if ip := net.ParseIP(host); ip != nil {
		if forbiddenIP(ip) {
			return fmt.Errorf("refusing private or reserved address %q", host)
		}
		return nil
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("cannot resolve %q: %w", host, err)
	}
	if len(addrs) == 0 {
		return fmt.Errorf("cannot resolve %q", host)
	}
	for _, addr := range addrs {
		if forbiddenIP(addr.IP) {
			return fmt.Errorf("refusing private or reserved address %q", addr.IP.String())
		}
	}
	return nil
}

func forbiddenIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsUnspecified() || ip.IsLoopback() || ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsInterfaceLocalMulticast() {
		return true
	}
	if v4 := ip.To4(); v4 != nil {
		switch {
		case v4[0] == 0, v4[0] == 10, v4[0] == 127:
			return true
		case v4[0] == 169 && v4[1] == 254:
			return true
		case v4[0] == 172 && v4[1] >= 16 && v4[1] <= 31:
			return true
		case v4[0] == 192 && v4[1] == 168:
			return true
		case v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127:
			return true // carrier-grade NAT
		case v4[0] >= 224:
			return true // multicast + reserved
		}
	}
	return false
}

func normalizeSource(source string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case SourceHuggingFace, "hf":
		return SourceHuggingFace, nil
	case "", SourceModelScope, "ms":
		return SourceModelScope, nil
	default:
		return "", fmt.Errorf("unsupported model source %q", source)
	}
}

func doJSON(ctx context.Context, method, target string, body io.Reader, out any) error {
	if err := ValidateURL(ctx, target); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "PopTrans/1.0")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("%s %s: HTTP %d %s", method, target, resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 8<<20))
	if err := decoder.Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// Search queries ModelScope or HuggingFace for GGUF models.
func Search(ctx context.Context, source, query string) ([]SearchResult, error) {
	source, err := normalizeSource(source)
	if err != nil {
		return nil, err
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("请输入搜索关键词")
	}
	var results []SearchResult
	if source == SourceHuggingFace {
		results, err = searchHuggingFace(ctx, query)
	} else {
		results, err = searchModelScope(ctx, query)
	}
	if err != nil {
		return nil, err
	}
	return enrichSearchResults(ctx, source, results), nil
}

// enrichSearchResults checks each repository's file list so only repos that
// actually contain GGUF files are shown, and replaces the size with the total
// GGUF size (more useful than the whole-repo storage size).
func enrichSearchResults(ctx context.Context, source string, results []SearchResult) []SearchResult {
	if len(results) == 0 {
		return results
	}
	type outcome struct {
		result SearchResult
		keep   bool
	}
	outcomes := make([]outcome, len(results))
	sem := make(chan struct{}, 6)
	var wait sync.WaitGroup
	for index, result := range results {
		wait.Add(1)
		go func(index int, result SearchResult) {
			defer wait.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				outcomes[index] = outcome{result: result, keep: true}
				return
			}
			files, err := Files(ctx, source, result.ID)
			if err != nil {
				// Keep the row with the search-provided size when the file list
				// cannot be fetched (network hiccup), rather than hiding it.
				outcomes[index] = outcome{result: result, keep: true}
				return
			}
			if len(files) == 0 {
				outcomes[index] = outcome{keep: false}
				return
			}
			var total int64
			for _, file := range files {
				total += file.Size
			}
			if total > 0 {
				result.SizeBytes = total
			}
			outcomes[index] = outcome{result: result, keep: true}
		}(index, result)
	}
	wait.Wait()

	enriched := make([]SearchResult, 0, len(results))
	for _, item := range outcomes {
		if item.keep {
			enriched = append(enriched, item.result)
		}
	}
	return enriched
}

func searchModelScope(ctx context.Context, query string) ([]SearchResult, error) {
	payload, err := json.Marshal(map[string]any{
		"PageSize":        20,
		"PageNumber":      1,
		"Name":            query,
		"SortBy":          "Default",
		"Target":          "",
		"SingleCriterion": []any{},
	})
	if err != nil {
		return nil, err
	}
	var response struct {
		Data struct {
			Model struct {
				Models []struct {
					Path        string `json:"Path"`
					Name        string `json:"Name"`
					Downloads   int64  `json:"Downloads"`
					Description string `json:"Description"`
					StorageSize int64  `json:"StorageSize"`
				} `json:"Models"`
			} `json:"Model"`
		} `json:"Data"`
	}
	target := modelScopeEndpoint + "/api/v1/dolphin/models"
	if err := doJSON(ctx, http.MethodPut, target, strings.NewReader(string(payload)), &response); err != nil {
		return nil, err
	}
	results := make([]SearchResult, 0, len(response.Data.Model.Models))
	for _, item := range response.Data.Model.Models {
		namespace := strings.TrimSpace(item.Path)
		name := strings.TrimSpace(item.Name)
		if namespace == "" || name == "" {
			continue
		}
		id := namespace + "/" + name
		results = append(results, SearchResult{
			ID:          id,
			Name:        id,
			Source:      SourceModelScope,
			Downloads:   item.Downloads,
			Description: strings.TrimSpace(item.Description),
			SizeBytes:   item.StorageSize,
			Parameters:  parameterSize(name),
		})
	}
	return results, nil
}

func searchHuggingFace(ctx context.Context, query string) ([]SearchResult, error) {
	target := huggingFaceEndpoint + "/api/models?search=" + url.QueryEscape(query) + "&filter=gguf&limit=20"
	var response []struct {
		ID        string   `json:"id"`
		Downloads int64    `json:"downloads"`
		Tags      []string `json:"tags"`
	}
	if err := doJSON(ctx, http.MethodGet, target, nil, &response); err != nil {
		return nil, err
	}
	results := make([]SearchResult, 0, len(response))
	for _, item := range response {
		if strings.TrimSpace(item.ID) == "" {
			continue
		}
		results = append(results, SearchResult{
			ID:         item.ID,
			Name:       item.ID,
			Source:     SourceHuggingFace,
			Downloads:  item.Downloads,
			Parameters: parameterSize(item.ID),
		})
	}
	return results, nil
}

// Files lists the .gguf files inside a repository.
func Files(ctx context.Context, source, repo string) ([]FileInfo, error) {
	source, err := normalizeSource(source)
	if err != nil {
		return nil, err
	}
	repo = strings.Trim(strings.TrimSpace(repo), "/")
	if repo == "" {
		return nil, errors.New("缺少模型仓库")
	}
	if source == SourceHuggingFace {
		return filesHuggingFace(ctx, repo)
	}
	return filesModelScope(ctx, repo)
}

func filesModelScope(ctx context.Context, repo string) ([]FileInfo, error) {
	target := modelScopeEndpoint + "/api/v1/models/" + escapeRepo(repo) + "/repo/files?Revision=master&Recursive=true"
	var response struct {
		Data struct {
			Files []struct {
				Path string `json:"Path"`
				Size int64  `json:"Size"`
				Type string `json:"Type"`
			} `json:"Files"`
		} `json:"Data"`
	}
	if err := doJSON(ctx, http.MethodGet, target, nil, &response); err != nil {
		return nil, err
	}
	files := make([]FileInfo, 0)
	for _, item := range response.Data.Files {
		name := strings.TrimSpace(item.Path)
		if item.Type != "" && item.Type != "blob" {
			continue
		}
		if !strings.EqualFold(filepath.Ext(name), ".gguf") {
			continue
		}
		files = append(files, FileInfo{Name: name, Size: item.Size})
	}
	return files, nil
}

func filesHuggingFace(ctx context.Context, repo string) ([]FileInfo, error) {
	target := huggingFaceEndpoint + "/api/models/" + escapeRepo(repo) + "/tree/main?recursive=true"
	var response []struct {
		Type string `json:"type"`
		Path string `json:"path"`
		Size int64  `json:"size"`
	}
	if err := doJSON(ctx, http.MethodGet, target, nil, &response); err != nil {
		return nil, err
	}
	files := make([]FileInfo, 0)
	for _, item := range response {
		if item.Type != "" && item.Type != "file" {
			continue
		}
		name := strings.TrimSpace(item.Path)
		if !strings.EqualFold(filepath.Ext(name), ".gguf") {
			continue
		}
		files = append(files, FileInfo{Name: name, Size: item.Size})
	}
	return files, nil
}

// DownloadURL builds the direct file URL for a repository file.
func DownloadURL(source, repo, file string) (string, error) {
	source, err := normalizeSource(source)
	if err != nil {
		return "", err
	}
	repo = strings.Trim(strings.TrimSpace(repo), "/")
	file = strings.Trim(strings.TrimSpace(strings.ReplaceAll(file, "\\", "/")), "/")
	if repo == "" || file == "" || strings.Contains(file, "..") {
		return "", errors.New("无效的模型文件")
	}
	if source == SourceHuggingFace {
		return huggingFaceEndpoint + "/" + escapeRepo(repo) + "/resolve/main/" + escapePath(file), nil
	}
	return modelScopeEndpoint + "/models/" + escapeRepo(repo) + "/resolve/master/" + escapePath(file), nil
}

// Download fetches a GGUF into dest, resuming a previous .part file when it
// exists. onProgress is called at most a few times per second.
func Download(
	ctx context.Context,
	source, repo, file, dest string,
	onProgress func(Progress),
) error {
	target, err := DownloadURL(source, repo, file)
	if err != nil {
		return err
	}
	if err := ValidateURL(ctx, target); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	partPath := dest + ".part"
	var offset int64
	if info, statErr := os.Stat(partPath); statErr == nil {
		offset = info.Size()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "PopTrans/1.0")
	req.Header.Set("Accept", "application/octet-stream")
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}
	client := &http.Client{
		CheckRedirect: httpClient.CheckRedirect,
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("下载失败: HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
	if resp.StatusCode == http.StatusOK {
		offset = 0 // server ignored the range request
	}
	total := resp.ContentLength
	if total > 0 {
		total += offset
	}

	flags := os.O_CREATE | os.O_WRONLY
	if offset == 0 {
		flags |= os.O_TRUNC
	} else {
		flags |= os.O_APPEND
	}
	handle, err := os.OpenFile(partPath, flags, 0o644)
	if err != nil {
		return err
	}
	defer handle.Close()

	written := offset
	lastReport := time.Now()
	buffer := make([]byte, 256*1024)
	for {
		n, readErr := resp.Body.Read(buffer)
		if n > 0 {
			if _, writeErr := handle.Write(buffer[:n]); writeErr != nil {
				return writeErr
			}
			written += int64(n)
			if onProgress != nil && time.Since(lastReport) > 200*time.Millisecond {
				lastReport = time.Now()
				onProgress(Progress{Downloaded: written, Total: total})
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			if onProgress != nil {
				onProgress(Progress{Downloaded: written, Total: total})
			}
			return readErr
		}
	}
	if onProgress != nil {
		onProgress(Progress{Downloaded: written, Total: total})
	}
	if err := handle.Close(); err != nil {
		return err
	}
	return os.Rename(partPath, dest)
}

func escapeRepo(repo string) string {
	parts := strings.Split(repo, "/")
	for index, part := range parts {
		parts[index] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func escapePath(path string) string {
	parts := strings.Split(path, "/")
	for index, part := range parts {
		parts[index] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

var parameterPattern = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*b(?:[^a-z0-9]|$)`)

// parameterSize extracts a display parameter count such as "1.8B" from a repo
// name. It returns "" when the name does not mention one.
func parameterSize(name string) string {
	match := parameterPattern.FindStringSubmatch(name)
	if len(match) < 2 {
		return ""
	}
	return match[1] + "B"
}
