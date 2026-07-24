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

func TestAutoApprovePrefersAllowOnce(t *testing.T) {
	// options 里 allow-always 在前时，仍应选 allow-once，避免会话被永久放行。
	params := acp.RequestPermissionRequest{
		SessionId: "s1",
		ToolCall:  acp.ToolCallUpdate{Title: titlePtr("Bash")},
		Options: []acp.PermissionOption{
			{OptionId: "allow-always", Kind: acp.PermissionOptionKindAllowAlways, Name: "Always"},
			{OptionId: "allow-once", Kind: acp.PermissionOptionKindAllowOnce, Name: "Once"},
		},
	}
	resp := autoApprovePermission(params)
	if resp.Outcome.Selected == nil || string(resp.Outcome.Selected.OptionId) != "allow-once" {
		t.Fatalf("expected allow-once, got %+v", resp.Outcome)
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

// TestCancelSessionClearsPending 验证 cancelSession 清除该会话的全部挂起权限：
// agent 进程退出/重连时调用此方法，避免残留死权限（接收方已失效，respond 永远无意义）。
// 这是会话变非活跃态后清理孤儿权限的后端机制（markSessionsError / ResumeSession 均依赖它）。
func TestCancelSessionClearsPending(t *testing.T) {
	b := newPermissionBroker()
	sid := acp.SessionId("s1")
	otherSid := acp.SessionId("s2")
	_ = b.registerWaiter(sid)
	defer b.unregisterWaiter(sid)
	_ = b.registerWaiter(otherSid)
	defer b.unregisterWaiter(otherSid)

	// 造两个 sid 上的挂起权限（request 会阻塞等响应，故在 goroutine 里发）
	ctx := context.Background()
	go func() {
		_, _ = b.request(ctx, acp.RequestPermissionRequest{
			SessionId: sid, ToolCall: acp.ToolCallUpdate{Title: titlePtr("Shell")},
			Options: []acp.PermissionOption{{OptionId: "allow-once", Kind: acp.PermissionOptionKindAllowOnce}},
		})
	}()
	go func() {
		_, _ = b.request(ctx, acp.RequestPermissionRequest{
			SessionId: otherSid, ToolCall: acp.ToolCallUpdate{Title: titlePtr("Shell")},
			Options: []acp.PermissionOption{{OptionId: "allow-once", Kind: acp.PermissionOptionKindAllowOnce}},
		})
	}()
	// 等两条权限都入 pending
	deadline := time.After(2 * time.Second)
	for len(b.pending) < 2 {
		select {
		case <-deadline:
			t.Fatalf("timeout waiting 2 pending, got %d", len(b.pending))
		default:
			time.Sleep(2 * time.Millisecond)
		}
	}

	// 取消 sid：其挂起权限应被清，otherSid 的应保留
	b.cancelSession(sid)
	for id, p := range b.pending {
		if p.sessionID == sid {
			t.Errorf("cancelSession 后 sid 的权限仍残留: %s", id)
		}
	}
	// otherSid 应仍有 1 条
	remaining := 0
	for _, p := range b.pending {
		if p.sessionID == otherSid {
			remaining++
		}
	}
	if remaining != 1 {
		t.Errorf("otherSid 的权限应保留 1 条，实际 %d", remaining)
	}
}
