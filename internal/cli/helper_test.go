package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Alesion30/gw/internal/finder"
	"github.com/Alesion30/gw/internal/git"
)

// stubFinder は選択 UI を出さずに固定の結果を返す。
type stubFinder struct {
	result finder.Result
	err    error
	items  []string // Find に渡された候補を記録する
}

func (s *stubFinder) Find(items []string, _ finder.Options) (finder.Result, error) {
	s.items = items
	return s.result, s.err
}

// newTestEnv は初期コミット済みの一時リポジトリと、それを指す env を作る。
func newTestEnv(t *testing.T) (root string, e *env, fdr *stubFinder) {
	t.Helper()

	// ~/.gitconfig と system の設定を読ませない。hooksPath や push の既定値といった
	// 手元の設定で結果が変わると、テストがマシンごとに落ちる
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)

	root = resolve(t, t.TempDir())

	runGit(t, root, "init", "-b", "main")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "user.name", "test")
	runGit(t, root, "config", "commit.gpgsign", "false")

	writeFile(t, filepath.Join(root, "README.md"), "# test\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "init")

	fdr = &stubFinder{result: finder.Result{Index: -1}}
	e = &env{
		git:     git.Client{Dir: root},
		finder:  fdr,
		confirm: func(string) bool { return true },
		stdout:  &bytes.Buffer{},
		cwd:     root,
	}

	return root, e, fdr
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// resolve は symlink を解決する。macOS の一時ディレクトリは /var → /private/var の
// symlink で、git が返すパスと文字列比較できなくなるため揃えておく。
func resolve(t *testing.T, path string) string {
	t.Helper()

	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func assertDirExists(t *testing.T, path string) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("%s does not exist: %v", path, err)
	}
	if !info.IsDir() {
		t.Fatalf("%s is not a directory", path)
	}
}

func assertNotExists(t *testing.T, path string) {
	t.Helper()

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("%s still exists", path)
	}
}
