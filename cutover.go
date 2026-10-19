package main

import (
	"context"
	"errors"
	"fmt"
)

type DNSWriter interface {
	ZoneID(context.Context, string) (string, error)
	UpsertCNAME(context.Context, RecordChange) error
}

type RecordChange struct {
	ZoneID string
	Name   string
	Target string
	TTL    int
	Key    string
}

type CutoverRequest struct {
	Action                 string `json:"action"`
	Domain                 string `json:"domain"`
	Name                   string `json:"name"`
	Target                 string `json:"target"`
	PreviousTarget         string `json:"previous_target"`
	TTL                    int    `json:"ttl"`
	DigitalAssetsReady     bool   `json:"digital_assets_ready"`
	SubscriberUpdatesReady bool   `json:"subscriber_updates_ready"`
	ContentProcessingReady bool   `json:"content_processing_ready"`
}

type CutoverResult struct {
	Action string `json:"action"`
	Name   string `json:"name"`
	Target string `json:"target"`
	TTL    int    `json:"ttl"`
}

var ErrNotReady = errors.New("delivery workflow is not ready")

func ApplyCutover(ctx context.Context, dns DNSWriter, req CutoverRequest) (CutoverResult, error) {
	if req.Domain == "" || req.Name == "" || req.TTL < 30 || req.TTL > 300 {
		return CutoverResult{}, errors.New("domain, name, and a ttl from 30 through 300 are required")
	}

	target := req.Target
	switch req.Action {
	case "cutover":
		if !req.DigitalAssetsReady || !req.SubscriberUpdatesReady || !req.ContentProcessingReady {
			return CutoverResult{}, ErrNotReady
		}
		if target == "" {
			return CutoverResult{}, errors.New("target is required for cutover")
		}
	case "rollback":
		target = req.PreviousTarget
		if target == "" {
			return CutoverResult{}, errors.New("previous_target is required for rollback")
		}
	default:
		return CutoverResult{}, errors.New("action must be cutover or rollback")
	}

	zoneID, err := dns.ZoneID(ctx, req.Domain)
	if err != nil {
		return CutoverResult{}, fmt.Errorf("resolve zone: %w", err)
	}
	change := RecordChange{
		ZoneID: zoneID,
		Name:   req.Name,
		Target: target,
		TTL:    req.TTL,
		Key:    req.Action + ":" + zoneID + ":" + req.Name + ":" + target,
	}
	if err := dns.UpsertCNAME(ctx, change); err != nil {
		return CutoverResult{}, fmt.Errorf("write record: %w", err)
	}
	return CutoverResult{Action: req.Action, Name: req.Name, Target: target, TTL: req.TTL}, nil
}
