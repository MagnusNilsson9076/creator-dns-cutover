package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestInfraiDNSRequestBoundary(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("authorization header missing")
		}
		switch calls {
		case 1:
			if r.Method != http.MethodGet || r.URL.Path != "/v1/dns/domain/get" || r.URL.Query().Get("domain") != "creator.example" {
				t.Errorf("unexpected zone request: %s %s", r.Method, r.URL.String())
			}
			io.WriteString(w, `{"ok":true,"data":{"zone_id":"zone-123"},"error":null,"metadata":{}}`)
		case 2:
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			io.WriteString(w, `{"ok":false,"data":null,"error":{"code":"","message":"retry later"},"metadata":{}}`)
		case 3:
			if r.Method != http.MethodPut || r.URL.Path != "/v1/dns/record/upsert" {
				t.Errorf("unexpected record request: %s %s", r.Method, r.URL.Path)
			}
			if r.Header.Get("Idempotency-Key") == "" {
				t.Errorf("idempotency key missing")
			}
			body, _ := io.ReadAll(r.Body)
			for _, field := range []string{`"zone_id":"zone-123"`, `"record_type":"CNAME"`, `"name":"media"`, `"content":"edge.new.example"`, `"ttl":60`} {
				if !strings.Contains(string(body), field) {
					t.Errorf("body %s does not contain %s", body, field)
				}
			}
			io.WriteString(w, `{"ok":true,"data":{},"error":null,"metadata":{}}`)
		}
	}))
	defer server.Close()

	client := &InfraiDNS{BaseURL: server.URL, APIKey: "test-key", HTTP: server.Client(), Sleep: func(context.Context, time.Duration) error { return nil }}
	zoneID, err := client.ZoneID(context.Background(), "creator.example")
	if err != nil || zoneID != "zone-123" {
		t.Fatalf("ZoneID() = %q, %v", zoneID, err)
	}
	err = client.UpsertCNAME(context.Background(), RecordChange{ZoneID: zoneID, Name: "media", Target: "edge.new.example", TTL: 60, Key: "change-1"})
	if err != nil {
		t.Fatalf("UpsertCNAME() error = %v", err)
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3", calls)
	}
}

func TestInfraiDNSDecodesRejectionBeforeStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{"ok":false,"data":null,"error":{"code":"","message":"check domain"},"metadata":{}}`)
	}))
	defer server.Close()

	client := &InfraiDNS{BaseURL: server.URL, APIKey: "test-key", HTTP: server.Client()}
	_, err := client.ZoneID(context.Background(), "creator.example")
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.Status != http.StatusBadRequest || apiErr.Message != "check domain" {
		t.Fatalf("error = %#v", err)
	}
}
