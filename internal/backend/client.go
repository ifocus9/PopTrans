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
	"strings"
	"time"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

type Health struct {
	Status           string         `json:"status"`
	TranslatorReady  bool           `json:"translator_ready"`
	TranslatorStatus string         `json:"translator_status"`
	OCRLoaded        bool           `json:"ocr_loaded"`
	DeviceInfo       map[string]any `json:"device_info,omitempty"`
}

type chatRequest struct {
	Messages   []chatMessage `json:"messages"`
	Stream     bool          `json:"stream"`
	SourceLang string        `json:"source_lang,omitempty"`
	TargetLang string        `json:"target_lang,omitempty"`
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

// GlossaryHit is a glossary term matched inside one OCR block.
type GlossaryHit struct {
	Term        string `json:"term"`
	Translation string `json:"translation"`
	Source      string `json:"source"`
	Note        string `json:"note"`
}

// OCRBlock is one OCR line with its translated text and pixel rectangle.
type OCRBlock struct {
	Text         string        `json:"text"`
	Translation  string        `json:"translation"`
	X            float64       `json:"x"`
	Y            float64       `json:"y"`
	W            float64       `json:"w"`
	H            float64       `json:"h"`
	Score        float64       `json:"score"`
	GlossaryHits []GlossaryHit `json:"glossary_hits"`
	// Rotated line geometry from OCR. Angle is degrees in screen coordinates
	// (negative = rising to the right); CX/CY is the line box centre;
	// LineWidth/LineHeight are the rotated box edge lengths.
	Angle      float64 `json:"angle"`
	CX         float64 `json:"cx"`
	CY         float64 `json:"cy"`
	LineWidth  float64 `json:"line_width"`
	LineHeight float64 `json:"line_height"`
}

// OCRLine is one recognized text line with its rotated geometry in image pixels.
type OCRLine struct {
	Text       string  `json:"text"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	W          float64 `json:"w"`
	H          float64 `json:"h"`
	Score      float64 `json:"score"`
	Angle      float64 `json:"angle"`
	CX         float64 `json:"cx"`
	CY         float64 `json:"cy"`
	LineWidth  float64 `json:"line_width"`
	LineHeight float64 `json:"line_height"`
}

// OCRBlocksResult is a pure OCR result: recognized text plus per-line geometry.
// Splitting OCR from translation lets the live loop OCR the next frame while
// the model translates the previous one.
type OCRBlocksResult struct {
	Text        string    `json:"text"`
	Lines       []string  `json:"lines"`
	LineBoxes   []OCRLine `json:"line_boxes"`
	ImageWidth  int       `json:"image_width"`
	ImageHeight int       `json:"image_height"`
	Scale       float64   `json:"scale"`
}

type translateBlocksRequest struct {
	Lines       []OCRLine `json:"lines"`
	SourceText  string    `json:"source_text"`
	ImageWidth  int       `json:"image_width"`
	ImageHeight int       `json:"image_height"`
	SourceLang  string    `json:"source_lang,omitempty"`
	TargetLang  string    `json:"target_lang,omitempty"`
	Force       bool      `json:"force,omitempty"`
}

// OCRTranslateBlocksResult returns per-line translations aligned to image boxes.
// ErrOCRBlocksUnsupported signals that the running AI engine predates the
// line-level blocks endpoint. Callers fall back to the legacy two-step path.
var ErrOCRBlocksUnsupported = errors.New("ocr translate blocks endpoint unsupported")

type OCRTranslateBlocksResult struct {
	SourceText  string     `json:"source_text"`
	Translation string     `json:"translation"`
	Blocks      []OCRBlock `json:"blocks"`
	ImageWidth  int        `json:"image_width"`
	ImageHeight int        `json:"image_height"`
	Scale       float64    `json:"scale"`
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			// OCR + LLM translation of a screenshot can exceed the old 35s cap,
			// especially on CPU. Individual callers still bound work via context.
			Timeout: 120 * time.Second,
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
	return c.TranslateWithLanguages(ctx, text, "", "")
}

// normalizeLanguage turns the UI's "auto" sentinel into an omitted form/JSON
// field; the backend treats a missing language as automatic.
func normalizeLanguage(value string) string {
	trimmed := strings.TrimSpace(value)
	if strings.EqualFold(trimmed, "auto") {
		return ""
	}
	return trimmed
}

// TranslateWithLanguages translates with explicit (or auto) source/target
// language codes.
func (c *Client) TranslateWithLanguages(ctx context.Context, text, sourceLang, targetLang string) (string, error) {
	sourceLang = normalizeLanguage(sourceLang)
	targetLang = normalizeLanguage(targetLang)
	startedAt := time.Now()
	log.Printf("backend translate request start: chars=%d source=%q target=%q preview=%q", len(text), sourceLang, targetLang, previewText(text, 120))
	payload := chatRequest{
		Messages: []chatMessage{{
			Role:    "user",
			Content: text,
		}},
		Stream:     false,
		SourceLang: sourceLang,
		TargetLang: targetLang,
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

	result := strings.TrimSpace(decoded.Choices[0].Message.Content)
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
	return c.OCRTranslateImageWithLanguages(ctx, pngBytes, "", "")
}

func (c *Client) OCRTranslateImageWithLanguages(ctx context.Context, pngBytes []byte, sourceLang, targetLang string) (OCRTranslateResult, error) {
	var decoded OCRTranslateResult
	startedAt := time.Now()
	log.Printf("backend ocr_translate request start: bytes=%d source=%q target=%q", len(pngBytes), sourceLang, targetLang)

	body, contentType, err := multipartImageBody(pngBytes, map[string]string{
		"source_lang": normalizeLanguage(sourceLang),
		"target_lang": normalizeLanguage(targetLang),
	})
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

// OCRTranslateBlocks recognizes an image and translates it line by line,
// preserving the OCR box for each line so the UI can overlay translations.
func (c *Client) OCRTranslateBlocks(ctx context.Context, pngBytes []byte) (OCRTranslateBlocksResult, error) {
	return c.OCRTranslateBlocksWithLanguages(ctx, pngBytes, "", "")
}

func (c *Client) OCRTranslateBlocksWithLanguages(ctx context.Context, pngBytes []byte, sourceLang, targetLang string) (OCRTranslateBlocksResult, error) {
	var decoded OCRTranslateBlocksResult
	startedAt := time.Now()
	log.Printf("backend ocr_translate_blocks request start: bytes=%d source=%q target=%q", len(pngBytes), sourceLang, targetLang)

	body, contentType, err := multipartImageBody(pngBytes, map[string]string{
		"source_lang": normalizeLanguage(sourceLang),
		"target_lang": normalizeLanguage(targetLang),
	})
	if err != nil {
		return decoded, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/ocr_translate_blocks", body)
	if err != nil {
		log.Printf("backend ocr_translate_blocks request build failed: %v", err)
		return decoded, err
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("backend ocr_translate_blocks request failed after %s: %v", time.Since(startedAt), err)
		return decoded, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		log.Printf("backend ocr_translate_blocks unsupported (404), legacy engine detected")
		return decoded, ErrOCRBlocksUnsupported
	}
	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		log.Printf("backend ocr_translate_blocks non-200 after %s: status=%s body=%s", time.Since(startedAt), resp.Status, string(payload))
		return decoded, fmt.Errorf("ocr translate blocks status: %s body: %s", resp.Status, string(payload))
	}

	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		log.Printf("backend ocr_translate_blocks decode failed after %s: %v", time.Since(startedAt), err)
		return decoded, err
	}

	log.Printf(
		"backend ocr_translate_blocks success after %s: blocks=%d source_chars=%d result_chars=%d",
		time.Since(startedAt),
		len(decoded.Blocks),
		len(decoded.SourceText),
		len(decoded.Translation),
	)
	return decoded, nil
}

// OCRBlocks recognizes an image and returns per-line geometry without
// translating it. On a 404 the running engine predates the split pipeline.
func (c *Client) OCRBlocks(ctx context.Context, pngBytes []byte) (OCRBlocksResult, error) {
	return c.OCRBlocksWithLanguages(ctx, pngBytes, "")
}

// OCRBlocksWithLanguages hints the OCR engine at the expected source language
// (e.g. "ko" selects the Korean recognition model).
func (c *Client) OCRBlocksWithLanguages(ctx context.Context, pngBytes []byte, sourceLang string) (OCRBlocksResult, error) {
	var decoded OCRBlocksResult
	startedAt := time.Now()
	log.Printf("backend ocr_blocks request start: bytes=%d source=%q", len(pngBytes), sourceLang)

	fields := map[string]string{
		// Live capture refreshes every frame; skip the slow second OCR pass.
		"allow_retry": "false",
	}
	if lang := normalizeLanguage(sourceLang); lang != "" {
		fields["source_lang"] = lang
	}
	body, contentType, err := multipartImageBody(pngBytes, fields)
	if err != nil {
		return decoded, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/ocr", body)
	if err != nil {
		log.Printf("backend ocr_blocks request build failed: %v", err)
		return decoded, err
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("backend ocr_blocks request failed after %s: %v", time.Since(startedAt), err)
		return decoded, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		log.Printf("backend ocr_blocks unsupported (404)")
		return decoded, ErrOCRBlocksUnsupported
	}
	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		log.Printf("backend ocr_blocks non-200 after %s: status=%s body=%s", time.Since(startedAt), resp.Status, string(payload))
		return decoded, fmt.Errorf("ocr status: %s body: %s", resp.Status, string(payload))
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		log.Printf("backend ocr_blocks decode failed after %s: %v", time.Since(startedAt), err)
		return decoded, err
	}
	if decoded.ImageWidth <= 0 || decoded.ImageHeight <= 0 {
		// Older engines omit the dimensions; the caller uses the capture rect.
		decoded.ImageWidth = 0
		decoded.ImageHeight = 0
	}

	log.Printf(
		"backend ocr_blocks success after %s: lines=%d chars=%d",
		time.Since(startedAt),
		len(decoded.LineBoxes),
		len(decoded.Text),
	)
	return decoded, nil
}

// TranslateBlocks translates lines that were already recognized by OCRBlocks.
func (c *Client) TranslateBlocks(ctx context.Context, lines []OCRLine, sourceText string, imageWidth, imageHeight int) (OCRTranslateBlocksResult, error) {
	return c.TranslateBlocksWithLanguages(ctx, lines, sourceText, imageWidth, imageHeight, "", "", false)
}

func (c *Client) TranslateBlocksWithLanguages(
	ctx context.Context,
	lines []OCRLine,
	sourceText string,
	imageWidth, imageHeight int,
	sourceLang, targetLang string,
	force bool,
) (OCRTranslateBlocksResult, error) {
	var decoded OCRTranslateBlocksResult
	startedAt := time.Now()
	log.Printf(
		"backend translate_blocks request start: lines=%d chars=%d source=%q target=%q force=%t",
		len(lines), len(sourceText), sourceLang, targetLang, force,
	)

	payload := translateBlocksRequest{
		Lines:       lines,
		SourceText:  sourceText,
		ImageWidth:  imageWidth,
		ImageHeight: imageHeight,
		SourceLang:  normalizeLanguage(sourceLang),
		TargetLang:  normalizeLanguage(targetLang),
		Force:       force,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return decoded, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/translate_blocks", bytes.NewReader(encoded))
	if err != nil {
		log.Printf("backend translate_blocks request build failed: %v", err)
		return decoded, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("backend translate_blocks request failed after %s: %v", time.Since(startedAt), err)
		return decoded, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		log.Printf("backend translate_blocks unsupported (404), legacy engine detected")
		return decoded, ErrOCRBlocksUnsupported
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("backend translate_blocks non-200 after %s: status=%s body=%s", time.Since(startedAt), resp.Status, string(body))
		return decoded, fmt.Errorf("translate blocks status: %s body: %s", resp.Status, string(body))
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		log.Printf("backend translate_blocks decode failed after %s: %v", time.Since(startedAt), err)
		return decoded, err
	}

	log.Printf(
		"backend translate_blocks success after %s: blocks=%d result_chars=%d",
		time.Since(startedAt),
		len(decoded.Blocks),
		len(decoded.Translation),
	)
	return decoded, nil
}

func multipartPNGBody(pngBytes []byte) (*bytes.Buffer, string, error) {
	return multipartImageBody(pngBytes, nil)
}

func multipartImageBody(pngBytes []byte, fields map[string]string) (*bytes.Buffer, string, error) {
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
	for key, value := range fields {
		if value == "" {
			continue
		}
		if err := writer.WriteField(key, value); err != nil {
			return nil, "", err
		}
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
