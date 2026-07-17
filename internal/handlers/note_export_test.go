package handlers

import (
	"strings"
	"testing"
)

func TestNoteFrontmatterRoundTrip(t *testing.T) {
	md := formatNoteMarkdown("周会纪要", "正文 #work", []string{"工作", "会议"})
	title, content, tags := parseNoteMarkdown(md)
	if title != "周会纪要" || !strings.Contains(content, "正文") {
		t.Fatalf("got title=%q content=%q tags=%v", title, content, tags)
	}
	if len(tags) < 2 {
		t.Fatalf("tags=%v", tags)
	}
	_, c2, _ := parseNoteMarkdown("纯正文\n第二行")
	if c2 != "纯正文\n第二行" {
		t.Fatalf("legacy content=%q", c2)
	}
}

func TestParseNoteTagsOnly(t *testing.T) {
	tags := parseNoteTags("#work #idea 其余内容")
	if len(tags) < 2 {
		t.Fatalf("tags=%v", tags)
	}
}
