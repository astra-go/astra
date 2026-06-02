//go:build mage

package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var semverRe = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.+\-]+)?$`)

// Release creates lockstep version tags for all workspace modules.
// Set VERSION env var to the desired version (e.g. VERSION=v1.0.0).
// Set DRY_RUN=1 to show tags without creating them.
// Set PUSH=1 to create and push tags in one step.
//
// Examples:
//
//	VERSION=v1.0.0 mage release
//	VERSION=v1.0.0 DRY_RUN=1 mage release
//	VERSION=v1.0.0 PUSH=1 mage release
func Release() error {
	version := os.Getenv("VERSION")
	if version == "" {
		return fmt.Errorf("VERSION env var is required (e.g. VERSION=v1.0.0 mage release)")
	}
	if !semverRe.MatchString(version) {
		return fmt.Errorf("invalid version %q — must be vMAJOR.MINOR.PATCH[-suffix]", version)
	}
	dryRun := os.Getenv("DRY_RUN") == "1"
	push := os.Getenv("PUSH") == "1"

	root, err := repoRoot()
	if err != nil {
		return err
	}

	fmt.Println("── 前置检查 ──")

	// 1. Working tree must be clean (unless dry-run).
	if !dryRun {
		out, _ := exec.Command("git", "status", "--porcelain").Output()
		if len(strings.TrimSpace(string(out))) > 0 {
			return fmt.Errorf("working tree has uncommitted changes — commit or stash first")
		}
	}

	// 2. Warn if not on main.
	branch, _ := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output()
	currentBranch := strings.TrimSpace(string(branch))
	if currentBranch != "main" && !dryRun {
		fmt.Printf("⚠  current branch is %q, not main\n", currentBranch)
		if !confirm("Continue?") {
			return fmt.Errorf("aborted")
		}
	}

	// 3. No intra-workspace replace directives in any go.mod.
	if err := checkNoIntraReplaces(root); err != nil {
		return err
	}

	// 4. Build tag list.
	fmt.Println()
	fmt.Println("── 计算 tag 列表 ──")
	mods, err := listModules(root, true)
	if err != nil {
		return err
	}

	tags := make([]string, 0, len(mods))
	for _, mod := range mods {
		if mod == "." {
			tags = append(tags, version)
		} else {
			tags = append(tags, mod+"/"+version)
		}
	}

	// 5. Check for existing tags.
	for _, tag := range tags {
		if err := exec.Command("git", "rev-parse", tag).Run(); err == nil {
			return fmt.Errorf("tag already exists: %s", tag)
		}
	}

	// 6. Show plan.
	fmt.Println()
	fmt.Printf("── 将创建 %d 个 tag ──\n", len(tags))
	for _, tag := range tags {
		fmt.Printf("  • %s\n", tag)
	}

	if dryRun {
		fmt.Println()
		fmt.Println("✓ dry-run 完成，未做任何改动")
		return nil
	}

	if !push {
		if !confirm(fmt.Sprintf("确认创建以上 %d 个 tag？", len(tags))) {
			fmt.Println("已取消")
			return nil
		}
	}

	// 7. Create tags.
	fmt.Println()
	fmt.Println("── 创建 tag ──")
	for _, tag := range tags {
		if err := exec.Command("git", "tag", "-a", tag, "-m", "Release "+tag).Run(); err != nil {
			return fmt.Errorf("git tag %s: %w", tag, err)
		}
		fmt.Printf("  ✓ created: %s\n", tag)
	}

	// 8. Push if requested.
	if push {
		fmt.Println()
		fmt.Println("── 推送 tag 到 origin ──")
		args := append([]string{"push", "origin"}, tags...)
		cmd := exec.Command("git", args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("git push: %w", err)
		}
		fmt.Println("✓ 推送完成")
	} else {
		fmt.Println()
		fmt.Println("✓ tag 已本地创建。手动推送命令：")
		fmt.Printf("  git push origin %s\n", strings.Join(tags, " "))
	}
	return nil
}

func checkNoIntraReplaces(root string) error {
	found := false
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			if d != nil && d.IsDir() && (d.Name() == ".git" || d.Name() == "vendor") {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "go.mod" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		inReplaceBlock := false
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "replace (") {
				inReplaceBlock = true
				continue
			}
			if inReplaceBlock && strings.HasPrefix(line, ")") {
				inReplaceBlock = false
				continue
			}
			if strings.HasPrefix(line, "replace ") || inReplaceBlock {
				// 只检查 intra-workspace replace（=> 后面是 ../）
				if strings.Contains(line, "=>") && strings.Contains(line, "../") {
					rel, _ := filepath.Rel(root, path)
					fmt.Fprintf(os.Stderr, "✗ intra-workspace replace found: %s\n", rel)
					found = true
					break
				}
			}
		}
		return nil
	})
	if found {
		return fmt.Errorf("run 'bash scripts/drop-intra-replaces.sh' to clean up before releasing")
	}
	return nil
}

// Auto-confirm for non-interactive use (CI/CD)
func confirm(prompt string) bool {
	// Check env var for auto-confirm
	if os.Getenv("AUTO_CONFIRM") == "1" {
		fmt.Printf("%s [y/N] y (auto-confirmed)\n", prompt)
		return true
	}
	fmt.Printf("%s [y/N] ", prompt)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		ans := strings.ToLower(strings.TrimSpace(scanner.Text()))
		return ans == "y" || ans == "yes"
	}
	return false
}
