package services

import (
	"reflect"
	"testing"
)

func TestParseClassifyTags(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{`["work","idea"]`, []string{"work", "idea"}},
		{"```json\n[\"todo\"]\n```", []string{"todo"}},
		{"标签：[\"学习\", \"工作\"]", []string{"学习", "工作"}},
		{"invalid", nil},
	}
	for _, tc := range tests {
		got := parseClassifyTags(tc.in)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("parseClassifyTags(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestParseClassifyResult(t *testing.T) {
	tags, title, ok := parseClassifyResult(`{"tags":["工作"],"title":"周会"}`)
	if !ok || title != "周会" || !reflect.DeepEqual(tags, []string{"工作"}) {
		t.Fatalf("object: tags=%v title=%q ok=%v", tags, title, ok)
	}
	tags, title, ok = parseClassifyResult("```json\n{\"tags\":[\"todo\"],\"title\":\"清单\"}\n```")
	if !ok || title != "清单" || !reflect.DeepEqual(tags, []string{"todo"}) {
		t.Fatalf("fenced: tags=%v title=%q ok=%v", tags, title, ok)
	}
	tags, title, ok = parseClassifyResult(`["a","b"]`)
	if !ok || title != "" || !reflect.DeepEqual(tags, []string{"a", "b"}) {
		t.Fatalf("array: tags=%v title=%q ok=%v", tags, title, ok)
	}
	_, _, ok = parseClassifyResult("invalid")
	if ok {
		t.Fatal("invalid should not ok")
	}
	// Agent 偶发重复推送整段 JSON，拼接后仍应解析出第一段
	dup := `{"tags":["k8s"],"title":"kubectl 查看 Pod 扩展信息"}{"tags":["k8s"],"title":"kubectl 查看 Pod 扩展信息"}`
	tags, title, ok = parseClassifyResult(dup)
	if !ok || title != "kubectl 查看 Pod 扩展信息" || !reflect.DeepEqual(tags, []string{"k8s"}) {
		t.Fatalf("dup chunks: tags=%v title=%q ok=%v", tags, title, ok)
	}
}

func TestEffectiveClassifyPrompt(t *testing.T) {
	if got := EffectiveClassifyPrompt(""); got != DefaultNoteClassifyPrompt {
		t.Fatalf("empty: got unexpected prompt")
	}
	if got := EffectiveClassifyPrompt(legacyNoteClassifyPrompt); got != DefaultNoteClassifyPrompt {
		t.Fatalf("legacy: should upgrade to new default")
	}
	custom := "自定义 {{content}}"
	if got := EffectiveClassifyPrompt(custom); got != custom {
		t.Fatalf("custom: got %q", got)
	}
}

func TestFormatClassifySessionTitle(t *testing.T) {
	if got := FormatClassifySessionTitle(3, 12); got != "笔记分类 (3/12)" {
		t.Fatalf("got %q", got)
	}
	if got := FormatClassifySessionTitle(0, 0); got != "笔记分类 (0/0)" {
		t.Fatalf("got %q", got)
	}
}

func TestNormalizeClassifyIntervalMinutes(t *testing.T) {
	if got := NormalizeClassifyIntervalMinutes(0); got != DefaultClassifyIntervalMinutes {
		t.Fatalf("zero = %d, want %d", got, DefaultClassifyIntervalMinutes)
	}
	if got := NormalizeClassifyIntervalMinutes(10); got != 10 {
		t.Fatalf("10 = %d", got)
	}
	if got := NormalizeClassifyIntervalMinutes(9999); got != MaxClassifyIntervalMinutes {
		t.Fatalf("9999 = %d, want %d", got, MaxClassifyIntervalMinutes)
	}
}

func TestMergeTags(t *testing.T) {
	got := mergeTags([]string{"a", "b"}, []string{"b", "c"})
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("mergeTags = %v, want %v", got, want)
	}
}
