package acp

import (
	"testing"

	"opennexus/internal/config"
)

func TestPermissionRules_PriorityDenyOverAllow(t *testing.T) {
	r := PermissionRules{
		Mode:  config.PermissionModeNormal,
		Allow: []string{"Bash(git *)"},
		Deny:  []string{"Bash(git push *)"},
	}
	// 同时命中 allow 和 deny → deny 优先
	if got := r.Decide("Bash(git push origin)"); got != DecisionDeny {
		t.Errorf("deny 应优先于 allow，期望 Deny，实际 %d", got)
	}
	// 仅命中 allow（git status 不被 deny 覆盖）
	if got := r.Decide("Bash(git status)"); got != DecisionAllow {
		t.Errorf("命中 allow 期望 Allow，实际 %d", got)
	}
}

func TestPermissionRules_AllowOverAsk(t *testing.T) {
	r := PermissionRules{
		Mode:  config.PermissionModeNormal,
		Allow: []string{"Bash(ls:*)"},
		Ask:   []string{"Bash(ls:*)"}, // 故意重叠
	}
	// allow 优先于 ask
	if got := r.Decide("Bash(ls:-la)"); got != DecisionAllow {
		t.Errorf("allow 应优先于 ask，期望 Allow，实际 %d", got)
	}
}

func TestPermissionRules_AskForcesPromptEvenInYolo(t *testing.T) {
	r := PermissionRules{
		Mode: config.PermissionModeYolo,
		Ask:  []string{"Bash(rm *)"},
	}
	// yolo 模式下命中 ask 仍走询问（`*` 跨 `/`）
	if got := r.Decide("Bash(rm -rf /tmp/x)"); got != DecisionAsk {
		t.Errorf("yolo 下命中 ask 应走询问，期望 Ask，实际 %d", got)
	}
}

func TestPermissionRules_YoloAllowsUnmatched(t *testing.T) {
	r := PermissionRules{Mode: config.PermissionModeYolo}
	if got := r.Decide("Bash(anything)"); got != DecisionAllow {
		t.Errorf("yolo 未命中应放行，期望 Allow，实际 %d", got)
	}
}

func TestPermissionRules_NormalAsksUnmatched(t *testing.T) {
	r := PermissionRules{Mode: config.PermissionModeNormal}
	if got := r.Decide("Bash(anything)"); got != DecisionAsk {
		t.Errorf("normal 未命中应询问，期望 Ask，实际 %d", got)
	}
}

func TestPermissionRules_GlobAndCaseInsensitive(t *testing.T) {
	r := PermissionRules{
		Mode:  config.PermissionModeNormal,
		Allow: []string{"Bash(git status *)"},
	}
	cases := map[string]Decision{
		"Bash(git status repo)":  DecisionAllow, // glob 命中
		"bash(GIT STATUS repo)":  DecisionAllow, // 大小写不敏感
		"Bash(git status)":       DecisionAsk,   // 无空格，不匹配 status *
		"Bash(git diff repo)":    DecisionAsk,   // 不同命令
		"Bash(git status a/b/c)": DecisionAllow, // `*` 跨 `/`
	}
	for title, want := range cases {
		if got := r.Decide(title); got != want {
			t.Errorf("Decide(%q) = %d, 期望 %d", title, got, want)
		}
	}
}

func TestPermissionRules_EmptyTitle(t *testing.T) {
	r := PermissionRules{Mode: config.PermissionModeYolo, Allow: []string{"*"}}
	// 空 title 无法匹配，保守询问
	if got := r.Decide(""); got != DecisionAsk {
		t.Errorf("空 title 应询问，期望 Ask，实际 %d", got)
	}
	if got := r.Decide("   "); got != DecisionAsk {
		t.Errorf("空白 title 应询问，期望 Ask，实际 %d", got)
	}
}

func TestPermissionRules_InvalidGlobFallsBackToExact(t *testing.T) {
	// `[` 在 path.Match 中是非法模式，应回退到精确匹配（小写）
	r := PermissionRules{
		Mode:  config.PermissionModeNormal,
		Allow: []string{"[invalid"},
	}
	if got := r.Decide("[invalid"); got != DecisionAllow {
		t.Errorf("非法 glob 应回退精确匹配，期望 Allow，实际 %d", got)
	}
	if got := r.Decide("[other"); got != DecisionAsk {
		t.Errorf("非法 glob 不匹配时期望 Ask，实际 %d", got)
	}
}

func TestCleanRules(t *testing.T) {
	// 去空白与空项；全空返回 nil
	if got := cleanRules([]string{" Bash(ls) ", "", "  "}); len(got) != 1 || got[0] != "Bash(ls)" {
		t.Errorf("cleanRules 清洗错误: %#v", got)
	}
	if got := cleanRules([]string{"", "   "}); got != nil {
		t.Errorf("全空应返回 nil，实际 %#v", got)
	}
}

func TestPermissionModeConstants(t *testing.T) {
	if config.PermissionModeNormal != "normal" || config.PermissionModeYolo != "yolo" {
		t.Errorf("权限模式常量取值异常: normal=%q yolo=%q", config.PermissionModeNormal, config.PermissionModeYolo)
	}
}
