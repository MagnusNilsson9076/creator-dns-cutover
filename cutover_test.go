package main

import (
	"context"
	"errors"
	"testing"
)

type recordingDNS struct {
	zoneID string
	writes []RecordChange
}

func (d *recordingDNS) ZoneID(_ context.Context, _ string) (string, error) {
	return d.zoneID, nil
}

func (d *recordingDNS) UpsertCNAME(_ context.Context, change RecordChange) error {
	d.writes = append(d.writes, change)
	return nil
}

func TestApplyCutoverDecision(t *testing.T) {
	tests := []struct {
		name       string
		req        CutoverRequest
		wantTarget string
		wantErr    error
		wantWrites int
	}{
		{
			name: "ready workflow changes the creator hostname",
			req: CutoverRequest{Action: "cutover", Domain: "creator.example", Name: "media", Target: "edge.new.example", PreviousTarget: "edge.old.example", TTL: 60,
				DigitalAssetsReady: true, SubscriberUpdatesReady: true, ContentProcessingReady: true},
			wantTarget: "edge.new.example", wantWrites: 1,
		},
		{
			name: "unfinished content processing blocks DNS",
			req: CutoverRequest{Action: "cutover", Domain: "creator.example", Name: "media", Target: "edge.new.example", TTL: 60,
				DigitalAssetsReady: true, SubscriberUpdatesReady: true, ContentProcessingReady: false},
			wantErr: ErrNotReady, wantWrites: 0,
		},
		{
			name:       "rollback restores the previous delivery target",
			req:        CutoverRequest{Action: "rollback", Domain: "creator.example", Name: "media", PreviousTarget: "edge.old.example", TTL: 60},
			wantTarget: "edge.old.example", wantWrites: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dns := &recordingDNS{zoneID: "zone-123"}
			result, err := ApplyCutover(context.Background(), dns, tt.req)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if len(dns.writes) != tt.wantWrites {
				t.Fatalf("writes = %d, want %d", len(dns.writes), tt.wantWrites)
			}
			if tt.wantWrites == 1 {
				if result.Target != tt.wantTarget || dns.writes[0].Target != tt.wantTarget {
					t.Fatalf("target = %q, write target = %q", result.Target, dns.writes[0].Target)
				}
				if dns.writes[0].ZoneID != "zone-123" {
					t.Fatalf("zone id = %q", dns.writes[0].ZoneID)
				}
			}
		})
	}
}
