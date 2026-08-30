package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/Alesion30/gw/internal/finder"
	"github.com/Alesion30/gw/internal/git"
	"github.com/Alesion30/gw/internal/ui"
)

// setupScript は worktree 作成後に実行するスクリプトのファイル名。
const setupScript = ".gw-setup"

func newUseCmd() *cobra.Command {
	var base string

	cmd := &cobra.Command{
		Use:   "use [query]",
		Short: "Create a worktree for a branch",
		Long: "Pick a local branch that has no worktree yet and create one for it.\n" +
			"Remote-tracking branches already in the repository are offered too, as <remote>/<branch>.\n" +
			"Picking origin/feature/foo creates the local branch feature/foo with origin/feature/foo as its upstream.\n" +
			"Nothing is fetched, so run `git fetch` first to see branches added on the remote.\n" +
			"A query that matches no branch skips the picker and asks whether to create a branch with that name.\n" +
			"Worktrees are created under $GW_WORKTREE_DIR (default: <repo-root>/.worktrees).\n" +
			"If ." + "gw-setup exists at the repository root, it runs inside the new worktree.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			e, err := newEnv()
			if err != nil {
				return err
			}
			return runUse(e, firstArg(args), base)
		},
	}

	cmd.Flags().StringVar(&base, "base", "", "starting point for a new branch (default: current branch)")

	return cmd
}

// branchCandidate は worktree を作れるブランチ 1 件を表す。
type branchCandidate struct {
	branch   string // worktree で checkout するローカルブランチ名
	upstream string // 起点にして upstream に張るリモート追跡ブランチ。ローカルブランチなら空
}

// label は Finder に出す表示名を返す。リモート追跡ブランチは
// 複数のリモートに同名のブランチがあっても見分けられるよう、リモート名で修飾する。
func (c branchCandidate) label() string {
	if c.upstream != "" {
		return c.upstream
	}
	return c.branch
}

func runUse(e *env, query, base string) error {
	inUse, err := branchesInWorktree(e)
	if err != nil {
		return err
	}

	locals, err := e.git.LocalBranches()
	if err != nil {
		return err
	}

	remotes, err := e.git.RemoteBranches()
	if err != nil {
		return err
	}

	candidates := availableBranches(locals, remotes, inUse)
	labels := make([]string, len(candidates))
	for i, c := range candidates {
		labels[i] = c.label()
	}

	selected, typed := -1, query

	// 候補に 1 件もマッチしないクエリは新規ブランチを作る意図なので、Finder を挟まず作成確認へ進む
	if finder.HasMatch(labels, query) {
		res, err := e.finder.Find(labels, finder.Options{
			Prompt:       "branch> ",
			Query:        query,
			AllowNoMatch: true,
		})
		if err != nil {
			if errors.Is(err, finder.ErrAborted) {
				return errors.New("canceled")
			}
			return err
		}

		typed, selected = res.Query, res.Index
	}

	var target branchCandidate
	newBranch := false

	if selected >= 0 {
		target = candidates[selected]
	} else {
		// 既存ブランチが選ばれなかったときは、入力文字列で新規ブランチを作るか確認する
		if typed == "" {
			return errors.New("no branch selected")
		}
		// 候補から外したリモート追跡ブランチを名指しされた場合、そのままでは
		// origin/feature/foo という名前のローカルブランチを作ってしまうので、理由を示して止める
		if rb, ok := findRemoteBranch(remotes, typed); ok {
			if inUse[rb.Branch] {
				return fmt.Errorf("branch %q is already checked out in another worktree", rb.Branch)
			}
			return fmt.Errorf("branch %q already exists locally; run `gw use %s` instead", rb.Branch, rb.Branch)
		}
		if e.git.BranchExists(typed) {
			return fmt.Errorf("branch %q is already checked out in another worktree", typed)
		}
		if !e.confirm(fmt.Sprintf("Branch %q does not exist. Create it?", typed)) {
			return errors.New("canceled")
		}
		target, newBranch = branchCandidate{branch: typed}, true
	}

	root, err := e.git.RepoRoot()
	if err != nil {
		return err
	}

	baseDir := os.Getenv("GW_WORKTREE_DIR")
	if baseDir == "" {
		baseDir = filepath.Join(root, ".worktrees")
	}
	// リモート追跡ブランチでも、作成先はリモート名を除いたブランチ名で決める
	wtPath := filepath.Join(baseDir, target.branch)

	switch {
	case newBranch:
		if err := e.git.AddWorktreeNewBranch(wtPath, target.branch, base); err != nil {
			return err
		}
		ui.Infof("Created worktree for new branch %q at %s", target.branch, wtPath)
	case target.upstream != "":
		if err := e.git.AddWorktreeTracking(wtPath, target.branch, target.upstream); err != nil {
			return err
		}
		ui.Infof("Created worktree for branch %q tracking %s at %s", target.branch, target.upstream, wtPath)
	default:
		if err := e.git.AddWorktree(wtPath, target.branch); err != nil {
			return err
		}
		ui.Infof("Created worktree for branch %q at %s", target.branch, wtPath)
	}

	return runSetup(root, wtPath)
}

// branchesInWorktree は既にどこかの worktree で checkout されているブランチ名を集める。
func branchesInWorktree(e *env) (map[string]bool, error) {
	worktrees, err := e.git.Worktrees()
	if err != nil {
		return nil, err
	}

	inUse := make(map[string]bool, len(worktrees))
	for _, wt := range worktrees {
		if wt.Branch != "" {
			inUse[wt.Branch] = true
		}
	}
	return inUse, nil
}

// availableBranches は worktree を作れるブランチの候補を並べる。
// worktree 化していないローカルブランチのあとに、対応するローカルブランチをまだ持たない
// リモート追跡ブランチが続く。
func availableBranches(locals []string, remotes []git.RemoteBranch, inUse map[string]bool) []branchCandidate {
	existsLocally := make(map[string]bool, len(locals))
	candidates := make([]branchCandidate, 0, len(locals)+len(remotes))

	for _, b := range locals {
		existsLocally[b] = true
		if !inUse[b] {
			candidates = append(candidates, branchCandidate{branch: b})
		}
	}

	for _, r := range remotes {
		// 同名のローカルブランチがあるならそちらを選べばよい。
		// worktree 化済みのブランチも必ずローカルに存在するので、この 1 つで両方を外せる
		if existsLocally[r.Branch] {
			continue
		}
		candidates = append(candidates, branchCandidate{branch: r.Branch, upstream: r.Name()})
	}

	return candidates
}

// findRemoteBranch は <remote>/<branch> 形式の名前に一致するリモート追跡ブランチを探す。
func findRemoteBranch(remotes []git.RemoteBranch, name string) (git.RemoteBranch, bool) {
	for _, r := range remotes {
		if r.Name() == name {
			return r, true
		}
	}
	return git.RemoteBranch{}, false
}

// runSetup はリポジトリルートの .gw-setup を、作成した worktree 内で実行する。
func runSetup(root, wtPath string) error {
	script := filepath.Join(root, setupScript)
	if _, err := os.Stat(script); err != nil {
		return nil
	}

	ui.Infof("Running %s in %s ...", script, wtPath)

	cmd := exec.Command("sh", script)
	cmd.Dir = wtPath
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
