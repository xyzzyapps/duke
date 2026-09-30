package doc

import "testing"

// newLoaded returns a buffer with preset content for edit tests.
func newLoaded(text string) *Buffer {
	b := New()
	b.Load(text)
	return b
}

func TestNewBufferIsEmpty(t *testing.T) {
	b := New()
	if b.LineCount() != 1 {
		t.Fatalf("LineCount = %d, want 1", b.LineCount())
	}
	if b.Text() != "" {
		t.Fatalf("Text = %q, want empty", b.Text())
	}
	if b.RuneCount(0) != 0 {
		t.Fatalf("RuneCount = %d, want 0", b.RuneCount(0))
	}
}

func TestInsertSingleLine(t *testing.T) {
	b := newLoaded("hello world")
	end := b.Insert(Pos{0, 5}, ",")
	if got, want := b.Text(), "hello, world"; got != want {
		t.Fatalf("Text = %q, want %q", got, want)
	}
	if end != (Pos{0, 6}) {
		t.Fatalf("end = %+v, want {0 6}", end)
	}
	if !b.Dirty() {
		t.Fatal("buffer should be dirty after insert")
	}
}

func TestInsertEmptyStringIsNoop(t *testing.T) {
	b := newLoaded("abc")
	b.Insert(Pos{0, 1}, "")
	if b.Text() != "abc" {
		t.Fatalf("Text = %q", b.Text())
	}
	if b.CanUndo() {
		t.Fatal("empty insert must not be recorded")
	}
}

func TestInsertSplitsLines(t *testing.T) {
	b := newLoaded("alphaomega")
	end := b.Insert(Pos{0, 5}, "\nbeta\n")
	if got, want := b.Text(), "alpha\nbeta\nomega"; got != want {
		t.Fatalf("Text = %q, want %q", got, want)
	}
	if end != (Pos{2, 0}) {
		t.Fatalf("end = %+v, want {2 0}", end)
	}
	if b.LineCount() != 3 {
		t.Fatalf("LineCount = %d, want 3", b.LineCount())
	}
}

func TestInsertClampsOutOfRangePos(t *testing.T) {
	b := newLoaded("abc")
	b.Insert(Pos{99, 99}, "X")
	if got, want := b.Text(), "abcX"; got != want {
		t.Fatalf("Text = %q, want %q", got, want)
	}
}

func TestDeleteWithinLine(t *testing.T) {
	b := newLoaded("hello world")
	deleted := b.Delete(Pos{0, 5}, Pos{0, 6})
	if deleted != " " {
		t.Fatalf("deleted = %q, want space", deleted)
	}
	if got, want := b.Text(), "helloworld"; got != want {
		t.Fatalf("Text = %q, want %q", got, want)
	}
}

func TestDeleteAcrossLines(t *testing.T) {
	b := newLoaded("one\ntwo\nthree")
	deleted := b.Delete(Pos{0, 1}, Pos{2, 2})
	if deleted != "ne\ntwo\nth" {
		t.Fatalf("deleted = %q", deleted)
	}
	if got, want := b.Text(), "oree"; got != want {
		t.Fatalf("Text = %q, want %q", got, want)
	}
}

func TestDeleteReversedEndpoints(t *testing.T) {
	b := newLoaded("abcdef")
	deleted := b.Delete(Pos{0, 4}, Pos{0, 2})
	if deleted != "cd" {
		t.Fatalf("deleted = %q, want cd", deleted)
	}
	if got, want := b.Text(), "abef"; got != want {
		t.Fatalf("Text = %q, want %q", got, want)
	}
}

func TestDeleteDegenerateRangeIsNoop(t *testing.T) {
	b := newLoaded("abc")
	if deleted := b.Delete(Pos{0, 1}, Pos{0, 1}); deleted != "" {
		t.Fatalf("deleted = %q, want empty", deleted)
	}
	if b.CanUndo() {
		t.Fatal("degenerate delete must not be recorded")
	}
}

func TestDeleteJoinsLines(t *testing.T) {
	b := newLoaded("ab\ncd")
	deleted := b.Delete(Pos{0, 2}, Pos{1, 0})
	if deleted != "\n" {
		t.Fatalf("deleted = %q, want newline", deleted)
	}
	if got, want := b.Text(), "abcd"; got != want {
		t.Fatalf("Text = %q, want %q", got, want)
	}
}

func TestMoveLineUpDown(t *testing.T) {
	b := newLoaded("a\nb\nc")
	if !b.MoveLine(1, -1) {
		t.Fatal("MoveLine up should succeed")
	}
	if got, want := b.Text(), "b\na\nc"; got != want {
		t.Fatalf("Text = %q, want %q", got, want)
	}
	if !b.MoveLine(0, 1) {
		t.Fatal("MoveLine down should succeed")
	}
	if got, want := b.Text(), "a\nb\nc"; got != want {
		t.Fatalf("Text = %q, want %q", got, want)
	}
}

func TestMoveLineEdgesFail(t *testing.T) {
	b := newLoaded("a\nb")
	if b.MoveLine(0, -1) {
		t.Fatal("moving first line up must fail")
	}
	if b.MoveLine(1, 1) {
		t.Fatal("moving last line down must fail")
	}
	if b.LineCount() != 2 {
		t.Fatalf("LineCount = %d, want 2", b.LineCount())
	}
}

func TestUndoRedoInsert(t *testing.T) {
	b := newLoaded("start")
	b.Insert(Pos{0, 5}, "!")
	h, ok := b.Undo()
	if !ok {
		t.Fatal("Undo should succeed")
	}
	if b.Text() != "start" {
		t.Fatalf("after undo Text = %q", b.Text())
	}
	if h.Pos != (Pos{0, 5}) {
		t.Fatalf("hint = %+v, want {0 5}", h.Pos)
	}
	if !b.CanRedo() {
		t.Fatal("redo should be available")
	}
	if _, ok := b.Redo(); !ok {
		t.Fatal("Redo should succeed")
	}
	if b.Text() != "start!" {
		t.Fatalf("after redo Text = %q", b.Text())
	}
}

func TestUndoRedoMultilineDelete(t *testing.T) {
	orig := "one\ntwo\nthree"
	b := newLoaded(orig)
	b.Delete(Pos{0, 1}, Pos{2, 2})
	if b.Text() == orig {
		t.Fatal("delete should change text")
	}
	if _, ok := b.Undo(); !ok {
		t.Fatal("Undo should succeed")
	}
	if b.Text() != orig {
		t.Fatalf("after undo Text = %q, want %q", b.Text(), orig)
	}
	if _, ok := b.Redo(); !ok {
		t.Fatal("Redo should succeed")
	}
	if got, want := b.Text(), "oree"; got != want {
		t.Fatalf("after redo Text = %q, want %q", got, want)
	}
}

func TestUndoRedoMoveLine(t *testing.T) {
	b := newLoaded("a\nb\nc")
	if !b.MoveLine(0, 1) {
		t.Fatal("move should succeed")
	}
	h, ok := b.Undo()
	if !ok {
		t.Fatal("Undo should succeed")
	}
	if b.Text() != "a\nb\nc" {
		t.Fatalf("after undo Text = %q", b.Text())
	}
	if !h.KeepCol || h.Pos.Line != 0 {
		t.Fatalf("hint = %+v, want KeepCol at line 0", h)
	}
	h, ok = b.Redo()
	if !ok {
		t.Fatal("Redo should succeed")
	}
	if b.Text() != "b\na\nc" {
		t.Fatalf("after redo Text = %q", b.Text())
	}
	if !h.KeepCol || h.Pos.Line != 1 {
		t.Fatalf("redo hint = %+v, want KeepCol at line 1", h)
	}
}

func TestUndoEmptyReturnsFalse(t *testing.T) {
	b := New()
	if _, ok := b.Undo(); ok {
		t.Fatal("Undo on empty stack must fail")
	}
	if _, ok := b.Redo(); ok {
		t.Fatal("Redo on empty stack must fail")
	}
}

func TestEditClearsRedoStack(t *testing.T) {
	b := newLoaded("a")
	b.Insert(Pos{0, 1}, "b")
	b.Undo()
	if !b.CanRedo() {
		t.Fatal("redo expected after undo")
	}
	b.Insert(Pos{0, 1}, "c")
	if b.CanRedo() {
		t.Fatal("new edit must clear redo stack")
	}
}

func TestUndoSequence(t *testing.T) {
	b := New()
	b.Insert(Pos{0, 0}, "ab")      // "ab"
	b.Insert(Pos{0, 2}, "\ncd")    // "ab\ncd"
	b.Delete(Pos{0, 0}, Pos{0, 1}) // "b\ncd"
	for b.CanUndo() {
		if _, ok := b.Undo(); !ok {
			t.Fatal("Undo failed mid-sequence")
		}
	}
	if b.Text() != "" {
		t.Fatalf("after full undo Text = %q, want empty", b.Text())
	}
	for b.CanRedo() {
		if _, ok := b.Redo(); !ok {
			t.Fatal("Redo failed mid-sequence")
		}
	}
	if b.Text() != "b\ncd" {
		t.Fatalf("after full redo Text = %q, want %q", b.Text(), "b\ncd")
	}
}

func TestLoadNormalisesCRLFAndResetsState(t *testing.T) {
	b := newLoaded("dirty")
	b.Insert(Pos{0, 5}, "!")
	b.Load("a\r\nb\rc")
	if b.Text() != "a\nb\nc" {
		t.Fatalf("Text = %q", b.Text())
	}
	if b.Dirty() {
		t.Fatal("Load must clear dirty flag")
	}
	if b.CanUndo() || b.CanRedo() {
		t.Fatal("Load must clear undo/redo stacks")
	}
}

func TestClampAndAdvance(t *testing.T) {
	b := newLoaded("ab\ncd")
	if p := b.Clamp(Pos{-1, -1}); p != (Pos{0, 0}) {
		t.Fatalf("Clamp = %+v", p)
	}
	if p := b.Clamp(Pos{99, 99}); p != (Pos{1, 2}) {
		t.Fatalf("Clamp = %+v", p)
	}
	if p := b.Advance(Pos{0, 1}, "x\nyy"); p != (Pos{1, 2}) {
		t.Fatalf("Advance = %+v, want {1 2}", p)
	}
	// Advancing past the end clamps to the buffer end.
	if p := b.Advance(Pos{1, 2}, "z"); p != (Pos{1, 2}) {
		t.Fatalf("Advance past end = %+v, want {1 2}", p)
	}
}

func TestMarkSavedClearsDirty(t *testing.T) {
	b := newLoaded("x")
	if b.Dirty() {
		t.Fatal("fresh load must not be dirty")
	}
	b.Insert(Pos{0, 1}, "y")
	if !b.Dirty() {
		t.Fatal("edit must set dirty")
	}
	b.MarkSaved()
	if b.Dirty() {
		t.Fatal("MarkSaved must clear dirty")
	}
}

func TestRuneColumnsNotBytes(t *testing.T) {
	b := newLoaded("héllo")
	// 'é' is 2 bytes but 1 rune; insert after it at rune col 1.
	end := b.Insert(Pos{0, 1}, "X")
	if got, want := b.Text(), "hXéllo"; got != want {
		t.Fatalf("Text = %q, want %q", got, want)
	}
	if end != (Pos{0, 2}) {
		t.Fatalf("end = %+v, want {0 2}", end)
	}
	deleted := b.Delete(Pos{0, 2}, Pos{0, 3})
	if deleted != "é" {
		t.Fatalf("deleted = %q, want é", deleted)
	}
}
