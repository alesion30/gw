package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Alesion30/gw/internal/finder"
)

func TestRunUseExistingBranch(t *testing.T) {
	root, e, fdr := newTestEnv(t)
	runGit(t, root, "branch", "feat/login")

	fdr.result = finder.Result{Index: 0, Query: "feat"}

	if err := runUse(e, "", ""); err != nil {
		t.Fatalf("runUse() = %v", err)
	}

	// worktree 化済みの main は候補から外れる
	if want := []string{"feat/login"}; !equal(fdr.items, want) {
		t.Errorf("candidates = %v, want %v", fdr.items, want)
	}
	assertDirExists(t, filepath.Join(root, ".worktrees", "feat/login"))
}

func TestRunUseCreatesNewBranch(t *testing.T) {
	root, e, _ := newTestEnv(t)

	if err := runUse(e, "feat/new", ""); err != nil {
		t.Fatalf("runUse() = %v", err)
	}

	assertDirExists(t, filepath.Join(root, ".worktrees", "feat/new"))
	if !e.git.BranchExists("feat/new") {
		t.Error("the branch feat/new was not created")
	}
}

func TestRunUseSkipsFinderForUnmatchedQuery(t *testing.T) {
	root, e, fdr := newTestEnv(t)
	runGit(t, root, "branch", "feat/login")

	if err := runUse(e, "feat/new", ""); err != nil {
		t.Fatalf("runUse() = %v", err)
	}

	// 候補が残っていても、マッチしないクエリでは選択 UI を出さずに作成へ進む
	if fdr.items != nil {
		t.Errorf("the finder was called with %v, want no call", fdr.items)
	}
	assertDirExists(t, filepath.Join(root, ".worktrees", "feat/new"))
	if !e.git.BranchExists("feat/new") {
		t.Error("the branch feat/new was not created")
	}
}

func TestRunUseOpensFinderForMatchedQuery(t *testing.T) {
	root, e, fdr := newTestEnv(t)
	runGit(t, root, "branch", "feat/login")

	fdr.result = finder.Result{Index: 0, Query: "login"}

	if err := runUse(e, "login", ""); err != nil {
		t.Fatalf("runUse() = %v", err)
	}

	if want := []string{"feat/login"}; !equal(fdr.items, want) {
		t.Errorf("candidates = %v, want %v", fdr.items, want)
	}
	assertDirExists(t, filepath.Join(root, ".worktrees", "feat/login"))
}

func TestRunUseNewBranchDeclined(t *testing.T) {
	root, e, _ := newTestEnv(t)
	runGit(t, root, "branch", "feat/login")
	e.confirm = func(string) bool { return false }

	err := runUse(e, "feat/new", "")
	if err == nil || err.Error() != "canceled" {
		t.Fatalf("runUse() = %v, want canceled", err)
	}
	assertNotExists(t, filepath.Join(root, ".worktrees", "feat/new"))
	if e.git.BranchExists("feat/new") {
		t.Error("the branch feat/new was created")
	}
}

func TestRunUseNewBranchFromBase(t *testing.T) {
	root, e, fdr := newTestEnv(t)
	runGit(t, root, "branch", "feat/login")

	// main に 2 つ目のコミットを積み、1 つ目を起点に指定する
	first := strings.TrimSpace(runGit(t, root, "rev-parse", "HEAD"))
	writeFile(t, filepath.Join(root, "second.txt"), "second\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "second")

	if err := runUse(e, "feat/base", first); err != nil {
		t.Fatalf("runUse() = %v", err)
	}

	if fdr.items != nil {
		t.Errorf("the finder was called with %v, want no call", fdr.items)
	}

	wt := filepath.Join(root, ".worktrees", "feat/base")
	if got := strings.TrimSpace(runGit(t, wt, "rev-parse", "HEAD")); got != first {
		t.Errorf("HEAD = %s, want %s", got, first)
	}
}

func TestRunUseHonorsWorktreeDirEnv(t *testing.T) {
	_, e, _ := newTestEnv(t)

	base := resolve(t, t.TempDir())
	t.Setenv("GW_WORKTREE_DIR", base)

	if err := runUse(e, "feat/env", ""); err != nil {
		t.Fatalf("runUse() = %v", err)
	}
	assertDirExists(t, filepath.Join(base, "feat/env"))
}

func TestRunUseRunsSetupScript(t *testing.T) {
	root, e, _ := newTestEnv(t)
	writeFile(t, filepath.Join(root, setupScript), "#!/bin/sh\necho ran > .setup-ran\n")

	if err := runUse(e, "feat/setup", ""); err != nil {
		t.Fatalf("runUse() = %v", err)
	}

	marker := filepath.Join(root, ".worktrees", "feat/setup", ".setup-ran")
	if _, err := os.Stat(marker); err != nil {
		t.Errorf(".gw-setup did not run inside the worktree: %v", err)
	}
}

func TestRunUseRejectsBranchCheckedOutElsewhere(t *testing.T) {
	root, e, fdr := newTestEnv(t)
	runGit(t, root, "branch", "feat/taken")
	runGit(t, root, "worktree", "add", filepath.Join(root, ".worktrees", "feat/taken"), "feat/taken")

	// 候補にはもう出てこないため、同じ名前を打ち込んだ状況を作る
	fdr.result = finder.Result{Index: -1, Query: "feat/taken"}

	err := runUse(e, "feat/taken", "")
	if err == nil || !strings.Contains(err.Error(), "already checked out") {
		t.Fatalf("runUse() = %v, want already checked out error", err)
	}
}

func TestRunUseAborted(t *testing.T) {
	root, e, fdr := newTestEnv(t)
	runGit(t, root, "branch", "feat/a")

	fdr.err = finder.ErrAborted

	err := runUse(e, "", "")
	if err == nil || err.Error() != "canceled" {
		t.Fatalf("runUse() = %v, want canceled", err)
	}
}

func equal(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
