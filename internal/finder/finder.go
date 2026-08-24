// Package finder はインクリメンタル絞り込みの選択 UI を提供する。
package finder

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"unicode"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ErrAborted は Esc や Ctrl-C で選択を中断したときに返る。
var ErrAborted = errors.New("finder: aborted")

// Options は絞り込みの挙動を指定する。
type Options struct {
	Prompt string // プロンプト文字列（既定: "> "）
	Query  string // 初期クエリ
	Height int    // 候補の最大表示行数（既定: 10）

	// SelectOne は候補が 1 件のときに UI を出さずに確定する。
	SelectOne bool
	// AllowNoMatch はマッチ 0 件でも Enter を受け付け、入力文字列だけを返す。
	AllowNoMatch bool
}

// Result は選択結果を表す。Index が -1 のときは候補を選ばずに Query だけが確定した状態。
type Result struct {
	Index int
	Query string
}

// Finder は選択 UI の抽象。テストではスタブに差し替える。
type Finder interface {
	Find(items []string, opts Options) (Result, error)
}

// TUI は /dev/tty 上で動く既定の Finder。
type TUI struct{}

// Find は items から 1 件を選ばせる。
func (TUI) Find(items []string, opts Options) (Result, error) {
	return Find(items, opts)
}

const defaultHeight = 10

// Find は items を絞り込んで 1 件を選ばせる。
// 標準出力を選択結果の出力に使えるよう、UI の入出力は /dev/tty に直接つなぐ。
func Find(items []string, opts Options) (Result, error) {
	if len(items) == 0 {
		if opts.AllowNoMatch {
			return Result{Index: -1, Query: opts.Query}, nil
		}
		return Result{Index: -1}, ErrAborted
	}
	// fzf の --select-1 と同じく、クエリ適用後の候補が 1 件なら UI を出さずに確定する
	if opts.SelectOne {
		if matches := matchItems(items, opts.Query); len(matches) == 1 {
			return Result{Index: matches[0].Index, Query: opts.Query}, nil
		}
	}

	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return Result{Index: -1}, fmt.Errorf("cannot open the terminal: %w", err)
	}
	defer func() { _ = tty.Close() }()

	m := newModel(items, opts)
	p := tea.NewProgram(m, tea.WithInput(tty), tea.WithOutput(tty))

	final, err := p.Run()
	if err != nil {
		return Result{Index: -1}, err
	}

	res := final.(model)
	if res.aborted {
		return Result{Index: -1}, ErrAborted
	}
	return Result{Index: res.selected, Query: res.input.Value()}, nil
}

var (
	cursorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("13")).Bold(true)
	selectedStyle = lipgloss.NewStyle().Bold(true)
	matchStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	counterStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

type model struct {
	items   []string
	opts    Options
	input   textinput.Model
	matches []match

	cursor   int // matches 内での位置
	offset   int // 表示ウィンドウの先頭
	height   int
	selected int
	aborted  bool
	done     bool
}

func newModel(items []string, opts Options) model {
	ti := textinput.New()
	ti.Prompt = opts.Prompt
	if ti.Prompt == "" {
		ti.Prompt = "> "
	}
	ti.SetValue(opts.Query)
	ti.CursorEnd()
	ti.Focus()

	height := opts.Height
	if height <= 0 {
		height = defaultHeight
	}

	m := model{
		items:    items,
		opts:     opts,
		input:    ti,
		height:   height,
		selected: -1,
	}
	m.filter()
	return m
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	switch keyMsg.Type {
	case tea.KeyCtrlC, tea.KeyEsc:
		m.aborted = true
		m.done = true
		return m, tea.Quit

	case tea.KeyEnter:
		if len(m.matches) > 0 {
			m.selected = m.matches[m.cursor].Index
			m.done = true
			return m, tea.Quit
		}
		if m.opts.AllowNoMatch {
			m.selected = -1
			m.done = true
			return m, tea.Quit
		}
		return m, nil

	case tea.KeyUp, tea.KeyCtrlP:
		m.moveCursor(-1)
		return m, nil

	case tea.KeyDown, tea.KeyCtrlN:
		m.moveCursor(1)
		return m, nil
	}

	prev := m.input.Value()

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)

	if m.input.Value() != prev {
		m.filter()
	}
	return m, cmd
}

func (m model) View() string {
	// 確定後は UI を消してターミナルを元の状態に戻す
	if m.done {
		return ""
	}

	var b strings.Builder
	b.WriteString(m.input.View())
	b.WriteByte('\n')
	b.WriteString(counterStyle.Render(fmt.Sprintf("  %d/%d", len(m.matches), len(m.items))))
	b.WriteByte('\n')

	end := min(m.offset+m.height, len(m.matches))
	for i := m.offset; i < end; i++ {
		if i == m.cursor {
			b.WriteString(cursorStyle.Render("▶ "))
		} else {
			b.WriteString("  ")
		}
		b.WriteString(renderMatch(m.matches[i], i == m.cursor))
		b.WriteByte('\n')
	}

	return b.String()
}

// renderMatch はマッチした文字を強調して 1 行分を描画する。
func renderMatch(m match, isCursor bool) string {
	matched := make(map[int]bool, len(m.MatchedIndexes))
	for _, idx := range m.MatchedIndexes {
		matched[idx] = true
	}

	var b strings.Builder
	for i, r := range []rune(m.Str) {
		s := string(r)
		switch {
		case matched[i]:
			b.WriteString(matchStyle.Render(s))
		case isCursor:
			b.WriteString(selectedStyle.Render(s))
		default:
			b.WriteString(s)
		}
	}
	return b.String()
}

// filter は現在の入力で候補を絞り込み、カーソル位置を先頭に戻す。
func (m *model) filter() {
	m.matches = matchItems(m.items, m.input.Value())
	m.cursor = 0
	m.offset = 0
}

// match は絞り込みに残った候補 1 件を表す。
type match struct {
	Str            string
	Index          int   // 元の items でのインデックス
	MatchedIndexes []int // 強調表示する rune の位置
}

// matchItems は空白区切りの語をすべて含む候補だけを元の順序で返す。空クエリなら全件返す。
func matchItems(items []string, query string) []match {
	fields := strings.Fields(query)
	words := make([][]rune, len(fields))
	for i, field := range fields {
		words[i] = foldRunes(field)
	}

	matches := make([]match, 0, len(items))
	for i, item := range items {
		indexes, ok := matchWords(item, words)
		if !ok {
			continue
		}
		matches = append(matches, match{Str: item, Index: i, MatchedIndexes: indexes})
	}
	return matches
}

// matchWords は item がすべての語を含むかを判定し、含むなら一致した rune の位置を昇順で返す。
func matchWords(item string, words [][]rune) ([]int, bool) {
	runes := foldRunes(item)
	hit := make([]bool, len(runes))

	for _, word := range words {
		found := false
		for i := 0; i+len(word) <= len(runes); i++ {
			if !slices.Equal(runes[i:i+len(word)], word) {
				continue
			}
			found = true
			for j := range word {
				hit[i+j] = true
			}
		}
		if !found {
			return nil, false
		}
	}

	var indexes []int
	for i, ok := range hit {
		if ok {
			indexes = append(indexes, i)
		}
	}
	return indexes, true
}

// foldRunes は rune 単位で小文字化する。strings.ToLower は rune 数が変わり得て強調位置がずれる。
func foldRunes(s string) []rune {
	runes := []rune(s)
	for i, r := range runes {
		runes[i] = unicode.ToLower(r)
	}
	return runes
}

func (m *model) moveCursor(delta int) {
	if len(m.matches) == 0 {
		return
	}

	m.cursor = max(0, min(m.cursor+delta, len(m.matches)-1))

	switch {
	case m.cursor < m.offset:
		m.offset = m.cursor
	case m.cursor >= m.offset+m.height:
		m.offset = m.cursor - m.height + 1
	}
}
