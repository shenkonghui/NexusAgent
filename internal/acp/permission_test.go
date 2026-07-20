package acp

import (
	"context"
	"testing"
	"time"

	"github.com/coder/acp-go-sdk"
)

func titlePtr(s string) *string { return &s }

func TestTrustedMCPAutoApprove(t *testing.T) {
	b := newPermissionBroker()
	sid := acp.SessionId("s1")
	_ = b.registerWaiter(sid)
	defer b.unregisterWaiter(sid)

	title := "opennexus-notes-list_notes: list_notes"
	resp, err := b.request(context.Background(), acp.RequestPermissionRequest{
		SessionId: sid,
		ToolCall:  acp.ToolCallUpdate{Title: &title},
		Options: []acp.PermissionOption{{
			OptionId: "allow-once",
			Kind:     acp.PermissionOptionKindAllowOnce,
			Name:     "Allow once",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Outcome.Selected == nil || string(resp.Outcome.Selected.OptionId) != "allow-once" {
		t.Fatalf("expected auto allow-once, got %+v", resp.Outcome)
	}
	if len(b.pending) != 0 {
		t.Fatalf("trusted mcp should not leave pending, got %d", len(b.pending))
	}
}

func TestAllowAlwaysBatchesPending(t *testing.T) {
	b := newPermissionBroker()
	sid := acp.SessionId("s1")
	ch := b.registerWaiter(sid)
	defer b.unregisterWaiter(sid)

	ctx := context.Background()
	done := make(chan acp.RequestPermissionResponse, 3)
	for i := 0; i < 3; i++ {
		go func() {
			resp, err := b.request(ctx, acp.RequestPermissionRequest{
				SessionId: sid,
				ToolCall:  acp.ToolCallUpdate{Title: titlePtr("Shell")},
				Options: []acp.PermissionOption{{
					OptionId: "allow-always",
					Kind:     acp.PermissionOptionKindAllowAlways,
					Name:     "Allow always",
				}},
			})
			if err != nil {
				t.Errorf("request: %v", err)
				return
			}
			done <- resp
		}()
	}

	var firstID string
	deadline := time.After(2 * time.Second)
	for i := 0; i < 3; i++ {
		select {
		case pn := <-ch:
			if firstID == "" {
				firstID = pn.RequestID
			}
		case <-deadline:
			t.Fatal("timeout waiting permission notify")
		}
	}

	if err := b.respond(firstID, "allow-always", false); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		select {
		case resp := <-done:
			if resp.Outcome.Selected == nil || string(resp.Outcome.Selected.OptionId) != "allow-always" {
				t.Fatalf("expected allow-always, got %+v", resp.Outcome)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting batched response")
		}
	}
	if len(b.pending) != 0 {
		t.Fatalf("pending should be empty, got %d", len(b.pending))
	}
}
