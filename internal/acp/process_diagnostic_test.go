package acp

import (
	"strings"
	"testing"
)

// TestRingBuffer 验证 ringBuffer 只保留尾部 maxLen 字节的环形截断行为。
func TestRingBuffer(t *testing.T) {
	t.Run("容量内完整保留", func(t *testing.T) {
		rb := newRingBuffer(64)
		rb.Write([]byte("hello"))
		if got := rb.String(); got != "hello" {
			t.Fatalf("got %q, want %q", got, "hello")
		}
	})

	t.Run("超容量截断尾部", func(t *testing.T) {
		rb := newRingBuffer(10)
		rb.Write([]byte("0123456789ABCDEF")) // 16 字节，超过 10
		got := rb.String()
		if len(got) != 10 {
			t.Fatalf("长度 %d, 期望 10 (尾部): %q", len(got), got)
		}
		// 期望保留最后 10 字节
		want := "6789ABCDEF" // "0123456789ABCDEF" 的最后 10 字节
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("多次写入累积截断", func(t *testing.T) {
		rb := newRingBuffer(5)
		rb.Write([]byte("aaa"))
		rb.Write([]byte("bbb"))
		rb.Write([]byte("ccc"))
		// 写入 aaa|bbb|ccc 共 9 字节，保留最后 5 字节
		if got := rb.String(); got != "bbccc" {
			t.Fatalf("got %q, want %q", got, "bbccc")
		}
	})
}

// TestStderrTail 验证 stderrTail 的去空白与空 buffer 处理。
func TestStderrTail(t *testing.T) {
	t.Run("空 buffer 返回空", func(t *testing.T) {
		p := &Process{stderrBuf: nil}
		if got := p.stderrTail(); got != "" {
			t.Fatalf("期望空字符串, got %q", got)
		}
	})

	t.Run("去首尾空白", func(t *testing.T) {
		rb := newRingBuffer(64)
		rb.Write([]byte("\n\n  some error output  \n\n"))
		p := &Process{stderrBuf: rb}
		if got := p.stderrTail(); got != "some error output" {
			t.Fatalf("got %q, want %q", got, "some error output")
		}
	})
}

// TestUnresponsiveDiagnosis 验证「进程存活但无响应」的诊断消息。
func TestUnresponsiveDiagnosis(t *testing.T) {
	t.Run("无 stderr 输出时给出通用提示", func(t *testing.T) {
		p := &Process{stderrBuf: nil}
		got := p.unresponsiveDiagnosis()
		if !strings.Contains(got, "握手无响应") {
			t.Fatalf("期望提示无响应, got %q", got)
		}
		if !strings.Contains(got, "作业控制信号挂起") {
			t.Fatalf("期望提示可能被挂起, got %q", got)
		}
	})

	t.Run("有 stderr 时附带尾部输出", func(t *testing.T) {
		rb := newRingBuffer(64)
		rb.Write([]byte("Error: missing API key\n"))
		p := &Process{stderrBuf: rb}
		got := p.unresponsiveDiagnosis()
		if !strings.Contains(got, "Error: missing API key") {
			t.Fatalf("期望包含 stderr 内容, got %q", got)
		}
		if !strings.Contains(got, "stderr 尾部输出") {
			t.Fatalf("期望包含 stderr 提示语, got %q", got)
		}
	})
}
