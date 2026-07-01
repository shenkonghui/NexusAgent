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
