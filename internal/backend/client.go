package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"time"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

type Health struct {
	Status           string `json:"status"`
	TranslatorReady  bool   `json:"translator_ready"`
	TranslatorStatus string `json:"translator_status"`
	OCRLoaded        bool   `json:"ocr_loaded"`
}

type chatRequest struct {
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type ocrResponse struct {
	Text  string   `json:"text"`
	Lines []string `json:"lines"`
	Scale float64  `json:"scale"`
}

type OCRTranslateResult struct {
	SourceText  string   `json:"source_text"`
	Translation string   `json:"translation"`
	Lines       []string `json:"lines"`
	Scale       float64  `json:"scale"`
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 35 * time.Second,
		},
	}
}

func (c *Client) Health(ctx context.Context) (Health, error) {
	var health Health
	startedAt := time.Now()
	log.Printf("backend health request start: %s/health", c.baseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		log.Printf("backend health request build failed: %v", err)
		return health, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("backend health request failed after %s: %v", time.Since(startedAt), err)
		return health, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("backend health non-200 after %s: status=%s body=%s", time.Since(startedAt), resp.Status, string(body))
		return health, fmt.Errorf("health status: %s body: %s", resp.Status, string(body))
	}

	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		log.Printf("backend health decode failed after %s: %v", time.Since(startedAt), err)
		return health, err
	}

	log.Printf(
		"backend health ok after %s: status=%s translator_ready=%t translator_status=%q ocr_loaded=%t",
		time.Since(startedAt),
		health.Status,
		health.TranslatorReady,
		health.TranslatorStatus,
		health.OCRLoaded,
	)
	return health, nil
}

func (c *Client) Translate(ctx context.Context, text string) (string, error) {
	startedAt := time.Now()
	log.Printf("backend translate request start: chars=%d preview=%q", len(text), previewText(text, 120))
	payload := chatRequest{
		Messages: []chatMessage{{
			Role:    "user",
			Content: text,
		}},
		Stream: false,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("backend translate marshal failed: %v", err)
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		log.Printf("backend translate request build failed: %v", err)
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("backend translate request failed after %s: %v", time.Since(startedAt), err)
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("backend translate non-200 after %s: status=%s body=%s", time.Since(startedAt), resp.Status, string(body))
		return "", fmt.Errorf("translate status: %s body: %s", resp.Status, string(body))
	}

	var decoded chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		log.Printf("backend translate decode failed after %s: %v", time.Since(startedAt), err)
		return "", err
	}

	if len(decoded.Choices) == 0 {
		log.Printf("backend translate empty choices after %s", time.Since(startedAt))
		return "", errors.New("translate response missing choices")
	}

	result := decoded.Choices[0].Message.Content
	log.Printf("backend translate success after %s: result_chars=%d preview=%q", time.Since(startedAt), len(result), previewText(result, 120))
	return result, nil
}

func (c *Client) OCRImage(ctx context.Context, pngBytes []byte) (string, error) {
	startedAt := time.Now()
	log.Printf("backend ocr request start: bytes=%d", len(pngBytes))

	body, contentType, err := multipartPNGBody(pngBytes)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/ocr", body)
	if err != nil {
		log.Printf("backend ocr request build failed: %v", err)
		return "", err
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("backend ocr request failed after %s: %v", time.Since(startedAt), err)
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("backend ocr non-200 after %s: status=%s body=%s", time.Since(startedAt), resp.Status, string(body))
		return "", fmt.Errorf("ocr status: %s body: %s", resp.Status, string(body))
	}

	var decoded ocrResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		log.Printf("backend ocr decode failed after %s: %v", time.Since(startedAt), err)
		return "", err
	}

	log.Printf("backend ocr success after %s: chars=%d preview=%q", time.Since(startedAt), len(decoded.Text), previewText(decoded.Text, 120))
	return decoded.Text, nil
}

func (c *Client) OCRTranslateImage(ctx context.Context, pngBytes []byte) (OCRTranslateResult, error) {
	var decoded OCRTranslateResult
	startedAt := time.Now()
	log.Printf("backend ocr_translate request start: bytes=%d", len(pngBytes))

	body, contentType, err := multipartPNGBody(pngBytes)
	if err != nil {
		return decoded, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/ocr_translate", body)
	if err != nil {
		log.Printf("backend ocr_translate request build failed: %v", err)
		return decoded, err
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("backend ocr_translate request failed after %s: %v", time.Since(startedAt), err)
		return decoded, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("backend ocr_translate non-200 after %s: status=%s body=%s", time.Since(startedAt), resp.Status, string(body))
		return decoded, fmt.Errorf("ocr translate status: %s body: %s", resp.Status, string(body))
	}

	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		log.Printf("backend ocr_translate decode failed after %s: %v", time.Since(startedAt), err)
		return decoded, err
	}

	log.Printf(
		"backend ocr_translate success after %s: source_chars=%d result_chars=%d source_preview=%q result_preview=%q",
		time.Since(startedAt),
		len(decoded.SourceText),
		len(decoded.Translation),
		previewText(decoded.SourceText, 80),
		previewText(decoded.Translation, 80),
	)
	return decoded, nil
}

func multipartPNGBody(pngBytes []byte) (*bytes.Buffer, string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="file"; filename="screenshot.png"`)
	header.Set("Content-Type", "image/png")
	part, err := writer.CreatePart(header)
	if err != nil {
		return nil, "", err
	}
	if _, err := part.Write(pngBytes); err != nil {
		return nil, "", err
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return &body, writer.FormDataContentType(), nil
}

func previewText(text string, limit int) string {
	text = bytes.NewBufferString(text).String()
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "..."
}
