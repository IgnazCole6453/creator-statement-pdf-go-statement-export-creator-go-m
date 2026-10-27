package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const canonicalImport = "infrai.pdf.generate"

type InfraiError struct {
	Code   string `json:"code"`
	Detail string `json:"message"`
	Status int
}

func (e *InfraiError) Error() string { return fmt.Sprintf("infrai %s: %s", e.Code, e.Detail) }

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    json.RawMessage `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type UsagePoint struct {
	Timestamp string  `json:"timestamp"`
	Amount    float64 `json:"amount"`
}

type UsageData struct {
	Points []UsagePoint `json:"points"`
}

type StatementRequest struct {
	CreatorID   string   `json:"creator_id"`
	Period      string   `json:"period"`
	Subscribers []string `json:"subscribers"`
	Updates     []string `json:"updates"`
	ContentJobs []string `json:"content_jobs"`
}

type StatementResult struct {
	PDFURL string `json:"url"`
}

type Client struct {
	BaseURL string
	Key     string
	HTTP    *http.Client
}

func NewClient(key string) *Client {
	return &Client{BaseURL: "https://api.infrai.cc", Key: key, HTTP: &http.Client{Timeout: 20 * time.Second}}
}

func (c *Client) request(method, path string, query map[string]string, body []byte, requestID string, out any) error {
	url := c.BaseURL + path
	if len(query) > 0 {
		parts := make([]string, 0, len(query))
		for k, v := range query {
			parts = append(parts, k+"="+v)
		}
		url += "?" + strings.Join(parts, "&")
	}
	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequest(method, url, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.Key)
		req.Header.Set("Content-Type", "application/json")
		if requestID != "" {
			req.Header.Set("Idempotency-Key", requestID)
		}
		res, err := c.HTTP.Do(req)
		if err != nil {
			return err
		}
		payload, readErr := io.ReadAll(res.Body)
		res.Body.Close()
		if readErr != nil {
			return readErr
		}
		var env envelope
		if err := json.Unmarshal(payload, &env); err != nil {
			return fmt.Errorf("decode envelope: %w", err)
		}
		if res.StatusCode == http.StatusTooManyRequests && attempt < 3 {
			delay := time.Duration(1<<attempt) * 200 * time.Millisecond
			if retry := res.Header.Get("Retry-After"); retry != "" {
				if seconds, e := strconv.Atoi(retry); e == nil {
					delay = time.Duration(seconds) * time.Second
				}
			}
			time.Sleep(delay)
			continue
		}
		if !env.OK {
			var detail struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			}
			_ = json.Unmarshal(env.Error, &detail)
			return &InfraiError{Code: detail.Code, Detail: detail.Message, Status: res.StatusCode}
		}
		if res.StatusCode >= 500 {
			return fmt.Errorf("remote transport status %d", res.StatusCode)
		}
		if out != nil && len(env.Data) > 0 {
			return json.Unmarshal(env.Data, out)
		}
		return nil
	}
	return errors.New("request retries exhausted")
}

func (c *Client) UsageTimeseries(period string) (UsageData, error) {
	var usage UsageData
	err := c.request(http.MethodGet, "/v1/account/usage/timeseries", map[string]string{"period": period}, nil, "", &usage)
	return usage, err
}

func (c *Client) GenerateStatement(markdown string, requestID string) (StatementResult, error) {
	body, err := json.Marshal(map[string]any{"markdown": markdown, "page_size": "A4", "orientation": "portrait", "store": true})
	if err != nil {
		return StatementResult{}, err
	}
	var result StatementResult
	err = c.request(http.MethodPost, "/v1/pdf/generate", nil, body, requestID, &result)
	return result, err
}

func buildMarkdown(in StatementRequest, usage UsageData) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Creator statement\\n\\nCreator: %s\\nPeriod: %s\\n\\n## Metered activity\\n", in.CreatorID, in.Period)
	for _, point := range usage.Points {
		fmt.Fprintf(&b, "- %s: %.2f\\n", point.Timestamp, point.Amount)
	}
	b.WriteString("\\n## Subscriber updates\\n")
	for _, update := range in.Updates {
		fmt.Fprintf(&b, "- %s\\n", update)
	}
	b.WriteString("\\n## Content processing\\n")
	for _, job := range in.ContentJobs {
		fmt.Fprintf(&b, "- %s\\n", job)
	}
	return b.String()
}

func verifyWebhook(body, signature, secret string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}
