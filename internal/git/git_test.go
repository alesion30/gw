package git

import (
	"reflect"
	"testing"
)

func TestParseWorktreeList(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want []Worktree
	}{
		{
			name: "empty",
			out:  "",
			want: nil,
		},
		{
			name: "main only",
			out:  "worktree /repo\nHEAD abc123\nbranch refs/heads/main\n",
			want: []Worktree{
				{Path: "/repo", Head: "abc123", Branch: "main"},
			},
		},
		{
			name: "multiple worktrees",
			out: "worktree /repo\nHEAD abc123\nbranch refs/heads/main\n\n" +
				"worktree /repo/.worktrees/feat/login\nHEAD def456\nbranch refs/heads/feat/login\n",
			want: []Worktree{
				{Path: "/repo", Head: "abc123", Branch: "main"},
				{Path: "/repo/.worktrees/feat/login", Head: "def456", Branch: "feat/login"},
			},
		},
		{
			name: "path containing spaces",
			out:  "worktree /repo/my project\nHEAD abc123\nbranch refs/heads/main\n",
			want: []Worktree{
				{Path: "/repo/my project", Head: "abc123", Branch: "main"},
			},
		},
		{
			name: "bare and detached",
			out: "worktree /repo.git\nbare\n\n" +
				"worktree /repo/wt\nHEAD abc123\ndetached\n",
			want: []Worktree{
				{Path: "/repo.git", Bare: true},
				{Path: "/repo/wt", Head: "abc123", Detached: true},
			},
		},
		{
			name: "locked and prunable",
			out:  "worktree /repo/wt\nHEAD abc123\nbranch refs/heads/x\nlocked reason here\nprunable gitdir file points to non-existent location\n",
			want: []Worktree{
				{Path: "/repo/wt", Head: "abc123", Branch: "x", Locked: true, Prunable: true},
			},
		},
		{
			name: "trailing blank lines",
			out:  "worktree /repo\nHEAD abc123\nbranch refs/heads/main\n\n\n",
			want: []Worktree{
				{Path: "/repo", Head: "abc123", Branch: "main"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseWorktreeList(tt.out)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseWorktreeList() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestParseGoneBranches(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want []string
	}{
		{
			name: "empty",
			out:  "",
			want: nil,
		},
		{
			name: "mixed tracking states",
			out: "main [behind 2]\n" +
				"feat/a [gone]\n" +
				"feat/b\n" +
				"feat/c [gone]\n" +
				"feat/d [ahead 1, behind 3]\n",
			want: []string{"feat/a", "feat/c"},
		},
		{
			name: "no gone branches",
			out:  "main\nfeat/a [ahead 1]\n",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseGoneBranches(tt.out)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseGoneBranches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseRemoteBranches(t *testing.T) {
	tests := []struct {
		name   string
		remote string
		out    string
		want   []RemoteBranch
	}{
		{
			name:   "empty",
			remote: "origin",
			out:    "",
			want:   nil,
		},
		{
			name:   "branches",
			remote: "origin",
			out: "refs/remotes/origin/feature/foo \n" +
				"refs/remotes/origin/main \n",
			want: []RemoteBranch{
				{Remote: "origin", Branch: "feature/foo"},
				{Remote: "origin", Branch: "main"},
			},
		},
		{
			name:   "symbolic ref",
			remote: "origin",
			out: "refs/remotes/origin/HEAD refs/remotes/origin/main\n" +
				"refs/remotes/origin/main \n",
			want: []RemoteBranch{
				{Remote: "origin", Branch: "main"},
			},
		},
		{
			name:   "another remote",
			remote: "upstream",
			out:    "refs/remotes/upstream/release/1.0 \n",
			want: []RemoteBranch{
				{Remote: "upstream", Branch: "release/1.0"},
			},
		},
		{
			name:   "prefix of another remote name",
			remote: "origin",
			out: "refs/remotes/origin2/main \n" +
				"refs/remotes/origin/main \n",
			want: []RemoteBranch{
				{Remote: "origin", Branch: "main"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseRemoteBranches(tt.remote, tt.out)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseRemoteBranches() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
