package assetflow

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type APIError struct {
	Code       string
	Message    string
	HTTPStatus int
}

func (e *APIError) Error() string {
	return fmt.Sprintf("infrai %s: %s", e.Code, e.Message)
}

type Client struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
	MaxRetries int
}

type envelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Hint    string `json:"hint"`
	} `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type PresignInput struct {
	Operation      string `json:"op"`
	ExpiresSeconds int    `json:"expires_seconds,omitempty"`
	ContentType    string `json:"content_type,omitempty"`
	MaxBytes       int64  `json:"max_bytes,omitempty"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

type PresignResult struct {
	URL string `json:"url"`
}

type HeadResult struct {
	Found bool `json:"found"`
}

func (c *Client) GetBucket(ctx context.Context, bucket string) error {
	return c.call(ctx, http.MethodGet, "/v1/storage/bucket/get/"+url.PathEscape(bucket), nil, nil)
}

func (c *Client) CreateBucket(ctx context.Context, name string) error {
	return c.call(ctx, http.MethodPost, "/v1/storage/bucket/create", map[string]string{"name": name}, nil)
}

// PresignObject is the Go call site for infrai.storage.object.presign.
func (c *Client) PresignObject(ctx context.Context, bucket, key string, input PresignInput) (PresignResult, error) {
	var result PresignResult
	err := c.call(ctx, http.MethodPost, "/v1/storage/object/presign/"+url.PathEscape(bucket)+"/"+url.PathEscape(key), input, &result)
	return result, err
}

func (c *Client) HeadObject(ctx context.Context, bucket, key string) (HeadResult, error) {
	var result HeadResult
	err := c.call(ctx, http.MethodGet, "/v1/storage/object/head/"+url.PathEscape(bucket)+"/"+url.PathEscape(key), nil, &result)
	return result, err
}

func (c *Client) call(ctx context.Context, method, path string, body, result any) error {
	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
	}

	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.BaseURL, "/")+path, bytes.NewReader(payload))
		if err != nil {
			return fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		res, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("send request: %w", err)
		}
		raw, readErr := io.ReadAll(res.Body)
		res.Body.Close()
		if readErr != nil {
			return fmt.Errorf("read response: %w", readErr)
		}

		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return fmt.Errorf("decode response envelope (HTTP %d): %w", res.StatusCode, err)
		}
		if res.StatusCode == http.StatusTooManyRequests && attempt < c.MaxRetries {
			delay := retryDelay(res.Header.Get("Retry-After"), attempt)
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
			continue
		}
		if !env.OK {
			apiErr := &APIError{HTTPStatus: res.StatusCode, Code: "request_rejected", Message: "request rejected"}
			if env.Error != nil {
				apiErr.Code = env.Error.Code
				apiErr.Message = env.Error.Message
				if apiErr.Message == "" {
					apiErr.Message = env.Error.Hint
				}
			}
			return apiErr
		}
		if res.StatusCode >= 500 {
			return fmt.Errorf("infrai HTTP %d", res.StatusCode)
		}
		if result != nil && len(env.Data) > 0 && string(env.Data) != "null" {
			if err := json.Unmarshal(env.Data, result); err != nil {
				return fmt.Errorf("decode response data: %w", err)
			}
		}
		return nil
	}
}

func retryDelay(header string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(header); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	delay := 100 * time.Millisecond * time.Duration(1<<attempt)
	if delay > 2*time.Second {
		return 2 * time.Second
	}
	return delay
}
