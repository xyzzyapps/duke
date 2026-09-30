package netplay

import "shooter/internal/doc"

// ObservedDoc wraps a doc.Document, forwarding every method and emitting a
// DocOp for each buffer mutation so the session layer can ship it to the
// host. Replays of remote ops (ApplyRemote) are suppressed: a received op
// must never be re-emitted (that would echo loops back to the host).
//
// Undo/redo forward to the inner document without emitting (their internal
// mutations bypass the wrapper by design); multiplayer disables them at
// the UI level anyway (their stacks interleave badly across peers).
//
// Not safe for concurrent use — the game loop owns it, and net messages
// are drained on the same goroutine.
type ObservedDoc struct {
	inner    doc.Document
	emit     func(DocOp)
	applying bool // suppress emission while replaying a remote op
}

// NewObservedDoc wraps d. emit may be nil (solo play = no-op observer).
func NewObservedDoc(d doc.Document, emit func(DocOp)) *ObservedDoc {
	return &ObservedDoc{inner: d, emit: emit}
}

// fire emits an op unless we are replaying or nobody is listening.
func (o *ObservedDoc) fire(op DocOp) {
	if o.applying || o.emit == nil {
		return
	}
	o.emit(op)
}

// ApplyRemote replays an op received from the network without re-emitting.
func (o *ObservedDoc) ApplyRemote(op DocOp) {
	o.applying = true
	defer func() { o.applying = false }()
	switch op.Kind {
	case "insert":
		o.inner.Insert(op.At, op.Text)
	case "delete":
		o.inner.Delete(op.At, op.End)
	case "moveline":
		o.inner.MoveLine(op.At.Line, op.Delta)
	case "load":
		o.inner.Load(op.Text)
	}
}

// Inner exposes the wrapped document (host snapshot / tests).
func (o *ObservedDoc) Inner() doc.Document { return o.inner }

// --- doc.Document forwarding ------------------------------------------------

// LineCount implements doc.Document.
func (o *ObservedDoc) LineCount() int { return o.inner.LineCount() }

// Line implements doc.Document.
func (o *ObservedDoc) Line(i int) string { return o.inner.Line(i) }

// RuneCount implements doc.Document.
func (o *ObservedDoc) RuneCount(line int) int { return o.inner.RuneCount(line) }

// Clamp implements doc.Document.
func (o *ObservedDoc) Clamp(p doc.Pos) doc.Pos { return o.inner.Clamp(p) }

// Advance implements doc.Document.
func (o *ObservedDoc) Advance(p doc.Pos, s string) doc.Pos { return o.inner.Advance(p, s) }

// Insert implements doc.Document and emits the op.
func (o *ObservedDoc) Insert(p doc.Pos, text string) doc.Pos {
	end := o.inner.Insert(p, text)
	o.fire(OpForInsert(p, text))
	return end
}

// Delete implements doc.Document and emits the op.
func (o *ObservedDoc) Delete(from, to doc.Pos) string {
	s := o.inner.Delete(from, to)
	if s != "" {
		o.fire(OpForDelete(from, to))
	}
	return s
}

// MoveLine implements doc.Document and emits the op.
func (o *ObservedDoc) MoveLine(line, delta int) bool {
	ok := o.inner.MoveLine(line, delta)
	if ok {
		o.fire(OpForMove(line, delta))
	}
	return ok
}

// Text implements doc.Document.
func (o *ObservedDoc) Text() string { return o.inner.Text() }

// Load implements doc.Document and emits a load op (full replacement).
func (o *ObservedDoc) Load(text string) {
	o.inner.Load(text)
	o.fire(OpForLoad(text))
}

// Dirty implements doc.Document.
func (o *ObservedDoc) Dirty() bool { return o.inner.Dirty() }

// MarkSaved implements doc.Document.
func (o *ObservedDoc) MarkSaved() { o.inner.MarkSaved() }

// CanUndo implements doc.Document.
func (o *ObservedDoc) CanUndo() bool { return o.inner.CanUndo() }

// CanRedo implements doc.Document.
func (o *ObservedDoc) CanRedo() bool { return o.inner.CanRedo() }

// Undo implements doc.Document (no emission: multiplayer disables undo).
func (o *ObservedDoc) Undo() (doc.Hint, bool) { return o.inner.Undo() }

// Redo implements doc.Document (no emission).
func (o *ObservedDoc) Redo() (doc.Hint, bool) { return o.inner.Redo() }

var _ doc.Document = (*ObservedDoc)(nil)
