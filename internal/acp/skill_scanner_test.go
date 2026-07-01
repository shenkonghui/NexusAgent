package acp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSkillAdditionalDirectories(t *testing.T) {
	root := t.TempDir()
	userSkills := filepath.Join(root, "user-skills")
	projectRoot := filepath.Join(root, "project")
	projectSkills := filepath.Join(projectRoot, ".claude", "skills")
	missing := filepath.Join(root, "missing")

	for _, dir := range []string{userSkills, projectSkills} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	dirs := SkillAdditionalDirectories(projectRoot, []string{userSkills, missing}, []string{".claude/skills", ".agents/skills"})
	if len(dirs) != 2 {
		t.Fatalf("期望 2 个 additional 目录，实际 %d: %v", len(dirs), dirs)
	}
	if dirs[0] != userSkills {
		t.Fatalf("user dir 异常: %q", dirs[0])
	}
	if dirs[1] != projectSkills {
		t.Fatalf("project dir 异常: %q", dirs[1])
	}

	// cwd 本身不应重复加入
	onlyCwd := SkillAdditionalDirectories(projectRoot, nil, []string{".claude/skills"})
	if len(onlyCwd) != 1 || onlyCwd[0] != projectSkills {
		t.Fatalf("project subdir 解析异常: %v", onlyCwd)
	}
}

func TestScanSkills_ConfiguredDirs(t *testing.T) {
	root := t.TempDir()
	userSkills := filepath.Join(root, "user-skills")
	projectRoot := filepath.Join(root, "project")
	projectSkills := filepath.Join(projectRoot, ".claude", "skills")
	if err := os.MkdirAll(filepath.Join(userSkills, "alpha"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(userSkills, "alpha", "SKILL.md"), []byte("---\nname: alpha\ndescription: user skill\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectSkills, "beta"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectSkills, "beta", "SKILL.md"), []byte("---\nname: beta\ndescription: project skill\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	skills := ScanSkills(projectRoot, []string{userSkills}, []string{".claude/skills"})
	if len(skills) != 2 {
		t.Fatalf("期望 2 个 skills，实际 %d", len(skills))
	}
	if skills[0].Name != "beta" || skills[0].Scope != "project" {
		t.Fatalf("project skill 异常: %+v", skills[0])
	}
	if skills[1].Name != "alpha" || skills[1].Scope != "user" {
		t.Fatalf("user skill 异常: %+v", skills[1])
	}
}

func TestScanSkills_SymlinkDir(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target-skill")
	userSkills := filepath.Join(root, "user-skills")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "SKILL.md"), []byte("---\nname: linked\ndescription: via symlink\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(userSkills, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(userSkills, "linked")); err != nil {
		t.Fatal(err)
	}

	skills := ScanSkills("", []string{userSkills}, nil)
	if len(skills) != 1 || skills[0].Name != "linked" {
		t.Fatalf("期望扫描符号链接 skill 目录，实际: %+v", skills)
	}
}

func TestScanSkills_NestedSubdirs(t *testing.T) {
	root := t.TempDir()
	userSkills := filepath.Join(root, "user-skills")
	nested := filepath.Join(userSkills, "group", "nested-skill")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "SKILL.md"), []byte("---\nname: nested-skill\ndescription: deep skill\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	skills := ScanSkills("", []string{userSkills}, nil)
	if len(skills) != 1 || skills[0].Name != "nested-skill" {
		t.Fatalf("期望扫描嵌套 skill，实际: %+v", skills)
	}
}

func TestScanSkills_EmptyCwdStillScansUserDirs(t *testing.T) {
	root := t.TempDir()
	userSkills := filepath.Join(root, "user-skills")
	if err := os.MkdirAll(filepath.Join(userSkills, "only-user"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(userSkills, "only-user", "SKILL.md"), []byte("---\nname: only-user\ndescription: ok\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	skills := ScanSkills("", []string{userSkills}, []string{".claude/skills"})
	if len(skills) != 1 || skills[0].Name != "only-user" {
		t.Fatalf("期望扫描用户目录 skills，实际: %+v", skills)
	}
}
