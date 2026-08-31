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

func TestRunUseCreatesWorktreeFromRemoteBranch(t *testing.T) {
	root, e, fdr := newTestEnv(t)
	addRemote(t, root, "origin", "feature/foo")

	fdr.result = finder.Result{Index: 0, Query: "origin/feature/foo"}

	if err := runUse(e, "origin/feature/foo", ""); err != nil {
		t.Fatalf("runUse() = %v", err)
	}

	// リモート追跡ブランチはリモート名で修飾して候補に並ぶ
	if want := []string{"origin/feature/foo"}; !equal(fdr.items, want) {
		t.Errorf("candidates = %v, want %v", fdr.items, want)
	}

	// 作成先も checkout するブランチも、リモート名を除いた名前になる
	wt := filepath.Join(root, ".worktrees", "feature/foo")
	assertDirExists(t, wt)
	if !e.git.BranchExists("feature/foo") {
		t.Error("the branch feature/foo was not created")
	}
	if e.git.BranchExists("origin/feature/foo") {
		t.Error("a branch named origin/feature/foo was created")
	}

	if got, want := upstreamOf(t, wt), "origin/feature/foo"; got != want {
		t.Errorf("upstream = %s, want %s", got, want)
	}
	if got, want := revParse(t, wt, "HEAD"), revParse(t, root, "origin/feature/foo"); got != want {
		t.Errorf("HEAD = %s, want %s", got, want)
	}
}

func TestRunUseDoesNotFetch(t *testing.T) {
	root, e, fdr := newTestEnv(t)
	remote := addRemote(t, root, "origin", "feature/foo")

	// fetch 後にリモートへ増えたブランチと、到達できなくなった URL を用意して、
	// gw use が手元のリモート追跡ブランチだけで完結することを確かめる
	runGit(t, remote, "branch", "feature/later", "main")
	runGit(t, root, "remote", "set-url", "origin", filepath.Join(root, "missing.git"))

	fdr.result = finder.Result{Index: 0}

	if err := runUse(e, "", ""); err != nil {
		t.Fatalf("runUse() = %v", err)
	}

	if want := []string{"origin/feature/foo"}; !equal(fdr.items, want) {
		t.Errorf("candidates = %v, want %v", fdr.items, want)
	}

	wt := filepath.Join(root, ".worktrees", "feature/foo")
	assertDirExists(t, wt)
	if got, want := upstreamOf(t, wt), "origin/feature/foo"; got != want {
		t.Errorf("upstream = %s, want %s", got, want)
	}
}

func TestRunUseExcludesSymbolicRemoteRefs(t *testing.T) {
	root, e, fdr := newTestEnv(t)
	addRemote(t, root, "origin", "feature/foo")
	runGit(t, root, "remote", "set-head", "origin", "main")

	// 前提: origin/HEAD がシンボリック参照として存在する
	runGit(t, root, "symbolic-ref", "refs/remotes/origin/HEAD")

	fdr.result = finder.Result{Index: 0}

	if err := runUse(e, "", ""); err != nil {
		t.Fatalf("runUse() = %v", err)
	}

	if want := []string{"origin/feature/foo"}; !equal(fdr.items, want) {
		t.Errorf("candidates = %v, want %v", fdr.items, want)
	}
}

func TestRunUseExcludesRemoteBranchWithLocalCounterpart(t *testing.T) {
	root, e, fdr := newTestEnv(t)
	addRemote(t, root, "origin", "feature/foo", "feature/bar")
	runGit(t, root, "branch", "feature/foo", "main")

	fdr.result = finder.Result{Index: 0}

	if err := runUse(e, "", ""); err != nil {
		t.Fatalf("runUse() = %v", err)
	}

	// ローカルに同名のブランチがあるリモート追跡ブランチは並べない。
	// worktree 化済みの main も、そのリモート追跡ブランチも同様に外れる
	if want := []string{"feature/foo", "origin/feature/bar"}; !equal(fdr.items, want) {
		t.Errorf("candidates = %v, want %v", fdr.items, want)
	}
}

func TestRunUseExcludesRemoteBranchCheckedOutInWorktree(t *testing.T) {
	root, e, fdr := newTestEnv(t)
	addRemote(t, root, "origin", "feature/foo", "feature/bar")
	runGit(t, root, "worktree", "add", "-b", "feature/foo", filepath.Join(root, ".worktrees", "feature/foo"), "main")

	fdr.result = finder.Result{Index: 0}

	if err := runUse(e, "", ""); err != nil {
		t.Fatalf("runUse() = %v", err)
	}

	if want := []string{"origin/feature/bar"}; !equal(fdr.items, want) {
		t.Errorf("candidates = %v, want %v", fdr.items, want)
	}
}

func TestRunUseDistinguishesSameBranchAcrossRemotes(t *testing.T) {
	root, e, fdr := newTestEnv(t)
	addRemote(t, root, "origin", "feature/foo")
	addRemote(t, root, "upstream", "feature/foo")

	// 同じ短縮名でもリモート名で見分けられる。2 件目の upstream 側を選ぶ
	fdr.result = finder.Result{Index: 1}

	if err := runUse(e, "", ""); err != nil {
		t.Fatalf("runUse() = %v", err)
	}

	if want := []string{"origin/feature/foo", "upstream/feature/foo"}; !equal(fdr.items, want) {
		t.Errorf("candidates = %v, want %v", fdr.items, want)
	}

	wt := filepath.Join(root, ".worktrees", "feature/foo")
	if got, want := upstreamOf(t, wt), "upstream/feature/foo"; got != want {
		t.Errorf("upstream = %s, want %s", got, want)
	}
	if got, want := revParse(t, wt, "HEAD"), revParse(t, root, "upstream/feature/foo"); got != want {
		t.Errorf("HEAD = %s, want %s", got, want)
	}
}

func TestRunUseRejectsRemoteBranchWithLocalCounterpart(t *testing.T) {
	root, e, _ := newTestEnv(t)
	addRemote(t, root, "origin", "feature/foo")
	runGit(t, root, "branch", "feature/foo", "main")

	// 候補から外れたリモート追跡ブランチを名指しされても、その名前のブランチは作らない
	err := runUse(e, "origin/feature/foo", "")
	if err == nil || !strings.Contains(err.Error(), `branch "feature/foo" already exists`) {
		t.Fatalf("runUse() = %v, want an error about the existing local branch", err)
	}
	if e.git.BranchExists("origin/feature/foo") {
		t.Error("a branch named origin/feature/foo was created")
	}
	assertNotExists(t, filepath.Join(root, ".worktrees", "origin"))
}

func TestRunUseRejectsRemoteBranchCheckedOutInWorktree(t *testing.T) {
	root, e, _ := newTestEnv(t)
	addRemote(t, root, "origin", "feature/foo")
	runGit(t, root, "worktree", "add", "-b", "feature/foo", filepath.Join(root, ".worktrees", "feature/foo"), "main")

	err := runUse(e, "origin/feature/foo", "")
	if err == nil || !strings.Contains(err.Error(), "already checked out") {
		t.Fatalf("runUse() = %v, want already checked out error", err)
	}
	if e.git.BranchExists("origin/feature/foo") {
		t.Error("a branch named origin/feature/foo was created")
	}
}

func TestRunUseCreatesNewBranchAlongsideRemotes(t *testing.T) {
	root, e, fdr := newTestEnv(t)
	addRemote(t, root, "origin", "feature/foo")

	if err := runUse(e, "feat/new", "main"); err != nil {
		t.Fatalf("runUse() = %v", err)
	}

	// リモート追跡ブランチが候補にあっても、マッチしないクエリは作成確認へ進む
	if fdr.items != nil {
		t.Errorf("the finder was called with %v, want no call", fdr.items)
	}
	assertDirExists(t, filepath.Join(root, ".worktrees", "feat/new"))
	if !e.git.BranchExists("feat/new") {
		t.Error("the branch feat/new was not created")
	}
}

// addRemote は root の複製をリモート name として登録し、branches のブランチを用意して fetch する。
// ネットワークを使わずに、リモート追跡ブランチだけが手元にある状態を作り、その複製のパスを返す。
func addRemote(t *testing.T, root, name string, branches ...string) string {
	t.Helper()

	remote := filepath.Join(resolve(t, t.TempDir()), name)
	runGit(t, root, "clone", "--quiet", root, remote)
	runGit(t, remote, "config", "user.email", "test@example.com")
	runGit(t, remote, "config", "user.name", "test")
	runGit(t, remote, "config", "commit.gpgsign", "false")

	// リモートごとにコミットを変えて、どのリモートから枝分かれしたか判別できるようにする
	for _, branch := range branches {
		runGit(t, remote, "checkout", "--quiet", "-b", branch, "main")
		writeFile(t, filepath.Join(remote, branch+".txt"), name+"\n")
		runGit(t, remote, "add", ".")
		runGit(t, remote, "commit", "-m", branch)
	}

	runGit(t, root, "remote", "add", name, remote)
	runGit(t, root, "fetch", "--quiet", name)

	return remote
}

// upstreamOf は worktree が checkout しているブランチの upstream を返す。
func upstreamOf(t *testing.T, dir string) string {
	t.Helper()

	return revParse(t, dir, "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
}

func revParse(t *testing.T, dir string, args ...string) string {
	t.Helper()

	return strings.TrimSpace(runGit(t, dir, append([]string{"rev-parse"}, args...)...))
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
