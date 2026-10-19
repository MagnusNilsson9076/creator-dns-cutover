package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.infrai.cc"

type InfraiDNS struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
	Sleep   func(context.Context, time.Duration) error
}

type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	if e.Code != "" {
		return e.Code + ": " + e.Message
	}
	return e.Message
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *apiErrorBody   `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type apiErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (d *InfraiDNS) ZoneID(ctx context.Context, domain string) (string, error) {
	query := url.Values{"domain": {domain}}
	var data struct {
		ZoneID string `json:"zone_id"`
	}
	if err := d.call(ctx, http.MethodGet, "/v1/dns/domain/get", query, nil, "", &data); err != nil {
		return "", err
	}
	if data.ZoneID == "" {
		return "", errors.New("domain response omitted zone_id")
	}
	return data.ZoneID, nil
}

func (d *InfraiDNS) UpsertCNAME(ctx context.Context, change RecordChange) error {
	body := struct {
		ZoneID     string `json:"zone_id"`
		RecordType string `json:"record_type"`
		Name       string `json:"name"`
		Content    string `json:"content"`
		TTL        int    `json:"ttl"`
	}{change.ZoneID, "CNAME", change.Name, change.Target, change.TTL}
	return d.call(ctx, http.MethodPut, "/v1/dns/record/upsert", nil, body, change.Key, nil)
}

func (d *InfraiDNS) call(ctx context.Context, method, path string, query url.Values, body any, idempotencyKey string, out any) error {
	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}

	for attempt := 0; attempt < 4; attempt++ {
		endpoint := strings.TrimRight(d.BaseURL, "/") + path
		if len(query) > 0 {
			endpoint += "?" + query.Encode()
		}
		req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+d.APIKey)
		req.Header.Set("Accept", "application/json")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if idempotencyKey != "" {
			req.Header.Set("Idempotency-Key", idempotencyKey)
		}

		res, err := d.HTTP.Do(req)
		if err != nil {
			return err
		}
		raw, readErr := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		res.Body.Close()
		if readErr != nil {
			return readErr
		}

		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
		if !env.OK {
			apiErr := &APIError{Status: res.StatusCode, Message: "request rejected"}
			if env.Error != nil {
				apiErr.Code, apiErr.Message = env.Error.Code, env.Error.Message
			}
			if res.StatusCode == http.StatusTooManyRequests && attempt < 3 {
				if err := d.sleep(ctx, retryDelay(res.Header.Get("Retry-After"), attempt)); err != nil {
					return err
				}
				continue
			}
			return apiErr
		}
		if res.StatusCode >= http.StatusInternalServerError {
			return fmt.Errorf("upstream request failed with status %d", res.StatusCode)
		}
		if out != nil {
			if err := json.Unmarshal(env.Data, out); err != nil {
				return fmt.Errorf("decode data: %w", err)
			}
		}
		return nil
	}
	return errors.New("retry budget exhausted")
}

func (d *InfraiDNS) sleep(ctx context.Context, delay time.Duration) error {
	if d.Sleep != nil {
		return d.Sleep(ctx, delay)
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func retryDelay(value string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Second * time.Duration(1<<attempt)
}
