// Package doc implements the text buffer the gunman edits.
//
// The buffer is a pure data model: it knows nothing about rendering, the
// agent or input. Every mutation (Insert, Delete, MoveLine) is recorded on an
// undo stack as the original operation, so Undo/Redo can replay and invert
// operations in linear order. All coordinates are zero-based line numbers and
// zero-based *rune* columns (never byte offsets).
//
// Invariants:
//   - lines is never empty (it always contains at least one, possibly
//     empty, line);
//   - every Pos handed to the public API is clamped before use, so callers
//     may pass stale positions after an edit without corrupting the buffer.
package doc

import "strings"

// Pos identifies a location in the buffer: a zero-based line index and a
// zero-based rune column. Col may equal the rune count of the line, which
// denotes the position just past the last rune (the classic insertion point).
type Pos struct {
	Line int
	Col  int
}

// Hint describes where a cursor-like caret should go after an undo or redo.
// When KeepCol is true only Pos.Line is meaningful and the caller should
// preserve its previous column (used for line moves, where the column of the
// moved line is unchanged).
type Hint struct {
	Pos     Pos
	KeepCol bool
}

// Document is the buffer interface consumed by the rest of the editor.
// It is mocked in tests of the UI and action layers.
type Document interface {
	// LineCount returns the number of lines; always >= 1.
	LineCount() int
	// Line returns line i as a string; out-of-range yields "".
	Line(i int) string
	// RuneCount returns the number of runes in line i.
	RuneCount(line int) int
	// Clamp restricts p to a valid location in the buffer.
	Clamp(p Pos) Pos
	// Advance moves p forward by the runes of s (treating '\n' as a line
	// break) and clamps the result.
	Advance(p Pos, s string) Pos

	// Insert writes text at p and returns the position just after the
	// inserted text. Text may contain newlines, which split the line.
	Insert(p Pos, text string) Pos
	// Delete removes the half-open range [from, to) and returns the removed
	// text. The order of the endpoints does not matter; a degenerate range
	// is a no-op returning "".
	Delete(from, to Pos) string
	// MoveLine swaps line one position up (-1) or down (+1). Larger
	// magnitudes are treated as their sign (single-step moves only).
	// Returns false when the line is already at the buffer edge.
	MoveLine(line, delta int) bool

	// Text returns the whole buffer joined with '\n'.
	Text() string
	// Load replaces the buffer content, clears the undo/redo stacks and
	// marks the buffer as saved. '\r\n' and '\r' are normalised to '\n'.
	Load(text string)

	// Dirty reports whether the buffer changed since the last MarkSaved.
	Dirty() bool
	// MarkSaved clears the dirty flag.
	MarkSaved()

	CanUndo() bool
	CanRedo() bool
	// Undo reverts the most recent edit, returning a hint for the caret.
	Undo() (Hint, bool)
	// Redo re-applies the most recently undone edit.
	Redo() (Hint, bool)
}

// opKind classifies a recorded edit.
type opKind int

const (
	opInsert opKind = iota
	opDelete
	opMove
)

// op is one recorded edit. For opInsert, at/end delimit the inserted text.
// For opDelete, at is the start of the removed range and text is the removed
// content (its end is recomputed by advancing). For opMove, at.Line is the
// line index before the move and delta is the signed single step.
type op struct {
	kind  opKind
	at    Pos
	end   Pos
	text  string
	delta int
}

// Buffer is the concrete Document. It is not safe for concurrent use; the
// editor is single-threaded (Ebitengine Update/Draw).
type Buffer struct {
	lines [][]rune
	undo  []op
	redo  []op
	dirty bool
}

// New returns an empty buffer containing a single empty line.
func New() *Buffer {
	return &Buffer{lines: [][]rune{{}}}
}

// LineCount implements Document.
func (b *Buffer) LineCount() int { return len(b.lines) }

// Line implements Document.
func (b *Buffer) Line(i int) string {
	if i < 0 || i >= len(b.lines) {
		return ""
	}
	return string(b.lines[i])
}

// RuneCount implements Document.
func (b *Buffer) RuneCount(line int) int {
	if line < 0 || line >= len(b.lines) {
		return 0
	}
	return len(b.lines[line])
}

// Clamp implements Document.
func (b *Buffer) Clamp(p Pos) Pos {
	if p.Line < 0 {
		p.Line = 0
	}
	if p.Line >= len(b.lines) {
		p.Line = len(b.lines) - 1
	}
	if p.Col < 0 {
		p.Col = 0
	}
	if n := len(b.lines[p.Line]); p.Col > n {
		p.Col = n
	}
	return p
}

// Advance implements Document.
func (b *Buffer) Advance(p Pos, s string) Pos {
	p = b.Clamp(p)
	for _, r := range s {
		if r == '\n' {
			p.Line++
			p.Col = 0
			if p.Line >= len(b.lines) {
				p.Line = len(b.lines) - 1
				p.Col = len(b.lines[p.Line])
			}
			continue
		}
		p.Col++
		p = b.Clamp(p)
	}
	return p
}

// Text implements Document.
func (b *Buffer) Text() string {
	var sb strings.Builder
	for i, l := range b.lines {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(string(l))
	}
	return sb.String()
}

// Load implements Document.
func (b *Buffer) Load(text string) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	raw := strings.Split(text, "\n")
	b.lines = make([][]rune, len(raw))
	for i, s := range raw {
		b.lines[i] = []rune(s)
	}
	b.undo, b.redo = nil, nil
	b.dirty = false
}

// Dirty implements Document.
func (b *Buffer) Dirty() bool { return b.dirty }

// MarkSaved implements Document.
func (b *Buffer) MarkSaved() { b.dirty = false }

// push records an edit and invalidates the redo stack.
func (b *Buffer) push(o op) {
	b.undo = append(b.undo, o)
	b.redo = nil
	b.dirty = true
}

// Insert implements Document.
func (b *Buffer) Insert(p Pos, text string) Pos {
	if text == "" {
		return b.Clamp(p)
	}
	p = b.Clamp(p)
	end := b.insertAt(p, text)
	b.push(op{kind: opInsert, at: p, end: end, text: text})
	return end
}

// insertAt performs the raw insertion without recording it.
func (b *Buffer) insertAt(p Pos, text string) Pos {
	segs := strings.Split(text, "\n")
	head := string(b.lines[p.Line][:p.Col])
	tail := string(b.lines[p.Line][p.Col:])
	if len(segs) == 1 {
		b.lines[p.Line] = []rune(head + segs[0] + tail)
		return Pos{Line: p.Line, Col: p.Col + len([]rune(segs[0]))}
	}
	b.lines[p.Line] = []rune(head + segs[0])
	// Splice in the remaining lines.
	out := make([][]rune, 0, len(b.lines)+len(segs)-1)
	out = append(out, b.lines[:p.Line+1]...)
	for _, s := range segs[1 : len(segs)-1] {
		out = append(out, []rune(s))
	}
	last := segs[len(segs)-1]
	out = append(out, []rune(last+tail))
	out = append(out, b.lines[p.Line+1:]...)
	b.lines = out
	return Pos{Line: p.Line + len(segs) - 1, Col: len([]rune(last))}
}

// Delete implements Document.
func (b *Buffer) Delete(from, to Pos) string {
	from = b.Clamp(from)
	to = b.Clamp(to)
	if from.Line > to.Line || (from.Line == to.Line && from.Col > to.Col) {
		from, to = to, from
	}
	if from == to {
		return ""
	}
	text := b.slice(from, to)
	b.deleteRange(from, to)
	b.push(op{kind: opDelete, at: from, text: text})
	return text
}

// slice returns the text in the half-open range [from, to).
func (b *Buffer) slice(from, to Pos) string {
	if from.Line == to.Line {
		return string(b.lines[from.Line][from.Col:to.Col])
	}
	var sb strings.Builder
	sb.WriteString(string(b.lines[from.Line][from.Col:]))
	for i := from.Line + 1; i < to.Line; i++ {
		sb.WriteByte('\n')
		sb.WriteString(string(b.lines[i]))
	}
	sb.WriteByte('\n')
	sb.WriteString(string(b.lines[to.Line][:to.Col]))
	return sb.String()
}

// deleteRange performs the raw deletion without recording it.
func (b *Buffer) deleteRange(from, to Pos) {
	if from.Line == to.Line {
		row := b.lines[from.Line]
		b.lines[from.Line] = append(row[:from.Col:from.Col], row[to.Col:]...)
		return
	}
	joined := make([]rune, 0, from.Col+len(b.lines[to.Line])-to.Col)
	joined = append(joined, b.lines[from.Line][:from.Col]...)
	joined = append(joined, b.lines[to.Line][to.Col:]...)
	out := make([][]rune, 0, len(b.lines)-(to.Line-from.Line))
	out = append(out, b.lines[:from.Line+1]...)
	out = append(out, b.lines[to.Line+1:]...)
	b.lines = out
	b.lines[from.Line] = joined
}

// MoveLine implements Document.
func (b *Buffer) MoveLine(line, delta int) bool {
	if delta == 0 {
		return false
	}
	if line < 0 || line >= len(b.lines) {
		return false
	}
	dir := 1
	if delta < 0 {
		dir = -1
	}
	target := line + dir
	if target < 0 || target >= len(b.lines) {
		return false
	}
	b.swapLines(line, target)
	b.push(op{kind: opMove, at: Pos{Line: line}, delta: dir})
	return true
}

// swapLines exchanges two adjacent lines.
func (b *Buffer) swapLines(a, c int) {
	if a < 0 || c < 0 || a >= len(b.lines) || c >= len(b.lines) {
		return
	}
	b.lines[a], b.lines[c] = b.lines[c], b.lines[a]
}

// CanUndo implements Document.
func (b *Buffer) CanUndo() bool { return len(b.undo) > 0 }

// CanRedo implements Document.
func (b *Buffer) CanRedo() bool { return len(b.redo) > 0 }

// Undo implements Document.
func (b *Buffer) Undo() (Hint, bool) {
	if len(b.undo) == 0 {
		return Hint{}, false
	}
	o := b.undo[len(b.undo)-1]
	b.undo = b.undo[:len(b.undo)-1]
	var h Hint
	switch o.kind {
	case opInsert:
		b.deleteRange(o.at, o.end)
		h = Hint{Pos: o.at}
	case opDelete:
		b.insertAt(o.at, o.text)
		h = Hint{Pos: b.Advance(o.at, o.text)}
	case opMove:
		b.swapLines(o.at.Line, o.at.Line+o.delta)
		h = Hint{Pos: Pos{Line: o.at.Line}, KeepCol: true}
	}
	b.redo = append(b.redo, o)
	b.dirty = true
	return h, true
}

// Redo implements Document.
func (b *Buffer) Redo() (Hint, bool) {
	if len(b.redo) == 0 {
		return Hint{}, false
	}
	o := b.redo[len(b.redo)-1]
	b.redo = b.redo[:len(b.redo)-1]
	var h Hint
	switch o.kind {
	case opInsert:
		end := b.insertAt(o.at, o.text)
		h = Hint{Pos: end}
	case opDelete:
		end := b.Advance(o.at, o.text)
		b.deleteRange(o.at, end)
		h = Hint{Pos: o.at}
	case opMove:
		b.swapLines(o.at.Line, o.at.Line+o.delta)
		h = Hint{Pos: Pos{Line: o.at.Line + o.delta}, KeepCol: true}
	}
	b.undo = append(b.undo, o)
	b.dirty = true
	return h, true
}
