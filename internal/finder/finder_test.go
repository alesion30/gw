package finder

import (
	"errors"
	"slices"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

var items = []string{"main", "feat/login", "feat/logout", "fix/crash"}

func TestFilterEmptyQueryKeepsOrder(t *testing.T) {
	m := newModel(items, Options{})

	if len(m.matches) != len(items) {
		t.Fatalf("matches = %d, want %d", len(m.matches), len(items))
	}
	for i, match := range m.matches {
		if match.Str != items[i] || match.Index != i {
			t.Errorf("matches[%d] = %+v, want %s (index %d)", i, match, items[i], i)
		}
	}
}

func TestFilterWithInitialQuery(t *testing.T) {
	m := newModel(items, Options{Query: "login"})

	if len(m.matches) != 1 {
		t.Fatalf("matches = %+v, want 1", m.matches)
	}
	if m.matches[0].Str != "feat/login" {
		t.Errorf("matches[0] = %q, want feat/login", m.matches[0].Str)
	}
}

func TestEnterReturnsOriginalIndex(t *testing.T) {
	m := newModel(items, Options{Query: "crash"})

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := next.(model)

	if !got.done || got.aborted {
		t.Fatalf("done = %v, aborted = %v", got.done, got.aborted)
	}
	// 絞り込み後の位置ではなく、元の items でのインデックスを返す
	if got.selected != 3 {
		t.Errorf("selected = %d, want 3", got.selected)
	}
}

func TestEnterWithNoMatch(t *testing.T) {
	tests := []struct {
		name         string
		allowNoMatch bool
		wantDone     bool
	}{
		{name: "confirms with AllowNoMatch", allowNoMatch: true, wantDone: true},
		{name: "ignores Enter without AllowNoMatch", allowNoMatch: false, wantDone: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newModel(items, Options{Query: "zzz-nomatch", AllowNoMatch: tt.allowNoMatch})
			if len(m.matches) != 0 {
				t.Fatalf("matches = %+v, want 0", m.matches)
			}

			next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			got := next.(model)

			if got.done != tt.wantDone {
				t.Errorf("done = %v, want %v", got.done, tt.wantDone)
			}
			if tt.wantDone {
				if got.selected != -1 {
					t.Errorf("selected = %d, want -1", got.selected)
				}
				if got.input.Value() != "zzz-nomatch" {
					t.Errorf("query = %q, want zzz-nomatch", got.input.Value())
				}
			}
		})
	}
}

func TestAbortKeys(t *testing.T) {
	for _, key := range []tea.KeyType{tea.KeyEsc, tea.KeyCtrlC} {
		m := newModel(items, Options{})

		next, _ := m.Update(tea.KeyMsg{Type: key})
		got := next.(model)

		if !got.aborted || !got.done {
			t.Errorf("key %v: aborted = %v, done = %v", key, got.aborted, got.done)
		}
	}
}

func TestMoveCursorClampsAndScrolls(t *testing.T) {
	m := newModel(items, Options{Height: 2})

	m.moveCursor(-1)
	if m.cursor != 0 {
		t.Errorf("moved above the first item: cursor = %d", m.cursor)
	}

	m.moveCursor(1)
	m.moveCursor(1)
	if m.cursor != 2 || m.offset != 1 {
		t.Errorf("cursor = %d, offset = %d, want 2 and 1", m.cursor, m.offset)
	}

	m.moveCursor(10)
	if m.cursor != len(items)-1 {
		t.Errorf("moved past the last item: cursor = %d", m.cursor)
	}
}

func TestTypingResetsCursor(t *testing.T) {
	m := newModel(items, Options{})
	m.moveCursor(2)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	got := next.(model)

	if got.cursor != 0 || got.offset != 0 {
		t.Errorf("cursor = %d, offset = %d, want 0 and 0", got.cursor, got.offset)
	}
	if len(got.matches) != 3 {
		t.Errorf("matches = %+v, want 3", got.matches)
	}
}

func TestFindWithEmptyItems(t *testing.T) {
	if _, err := Find(nil, Options{}); !errors.Is(err, ErrAborted) {
		t.Errorf("Find() = %v, want ErrAborted", err)
	}

	res, err := Find(nil, Options{Query: "typed", AllowNoMatch: true})
	if err != nil {
		t.Fatalf("Find() = %v", err)
	}
	if res.Index != -1 || res.Query != "typed" {
		t.Errorf("Find() = %+v, want {Index: -1, Query: typed}", res)
	}
}

func TestFindSelectOne(t *testing.T) {
	res, err := Find([]string{"only"}, Options{SelectOne: true})
	if err != nil {
		t.Fatalf("Find() = %v", err)
	}
	if res.Index != 0 {
		t.Errorf("Index = %d, want 0", res.Index)
	}
}

func TestFindSelectOneAfterQuery(t *testing.T) {
	// クエリで 1 件に絞れたら UI を出さずに確定する（fzf の --select-1 相当）
	res, err := Find(items, Options{Query: "crash", SelectOne: true})
	if err != nil {
		t.Fatalf("Find() = %v", err)
	}
	if res.Index != 3 {
		t.Errorf("Index = %d, want 3", res.Index)
	}
}

func TestMatchItems(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  []string
	}{
		{name: "empty query keeps every item", query: "", want: items},
		{name: "partial match", query: "log", want: []string{"feat/login", "feat/logout"}},
		{name: "no match", query: "zzz", want: nil},
		// あいまい検索なら f-l-i の飛び飛び一致で feat/login が残るが、単語一致では落とす
		{name: "non-contiguous characters do not match", query: "fli", want: nil},
		{name: "case-insensitive query", query: "LOGIN", want: []string{"feat/login"}},
		{name: "every word must appear", query: "feat log", want: []string{"feat/login", "feat/logout"}},
		{name: "item missing one word is dropped", query: "feat crash", want: nil},
		{name: "extra spaces are ignored", query: "  feat   out ", want: []string{"feat/logout"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := matchItems(items, tt.query)
			if len(matches) != len(tt.want) {
				t.Fatalf("matchItems() = %+v, want %v", matches, tt.want)
			}
			for i, match := range matches {
				if match.Str != tt.want[i] {
					t.Errorf("matches[%d] = %q, want %q", i, match.Str, tt.want[i])
				}
			}
		})
	}
}

func TestMatchItemsIgnoresItemCase(t *testing.T) {
	mixed := []string{"Feat/Login", "MAIN"}

	tests := []struct {
		query string
		want  string
	}{
		{query: "login", want: "Feat/Login"},
		{query: "LoGiN", want: "Feat/Login"},
		{query: "main", want: "MAIN"},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			matches := matchItems(mixed, tt.query)
			if len(matches) != 1 {
				t.Fatalf("matchItems(%q) = %+v, want 1", tt.query, matches)
			}
			if matches[0].Str != tt.want {
				t.Errorf("matchItems(%q) = %q, want %q", tt.query, matches[0].Str, tt.want)
			}
		})
	}
}

func TestMatchItemsMarksEveryOccurrence(t *testing.T) {
	tests := []struct {
		name  string
		item  string
		query string
		want  []int
	}{
		{name: "each occurrence of a word", item: "logo/login", query: "log", want: []int{0, 1, 2, 5, 6, 7}},
		{name: "each word of the query", item: "feat/login", query: "feat in", want: []int{0, 1, 2, 3, 8, 9}},
		{name: "rune positions instead of byte positions", item: "機能/ログイン", query: "ログ", want: []int{3, 4}},
		{name: "nothing on an empty query", item: "feat/login", query: "", want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := matchItems([]string{tt.item}, tt.query)
			if len(matches) != 1 {
				t.Fatalf("matchItems() = %+v, want 1", matches)
			}
			if !slices.Equal(matches[0].MatchedIndexes, tt.want) {
				t.Errorf("MatchedIndexes = %v, want %v", matches[0].MatchedIndexes, tt.want)
			}
		})
	}
}
