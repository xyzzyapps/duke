package ui

import (
	"fmt"
	"log"

	"shooter/internal/doc"
	"shooter/internal/render"
)

// Tab is one open buffer with its file target and the remembered caret
// (restored when the tab becomes active again).
type Tab struct {
	Doc   doc.Document
	Path  string
	Caret doc.Pos
}

// Tabs is the shell-owned tab strip, shared with the editor through
// Services.Tabs (the same pointer pattern as render.Layout). The editor
// mutates it; the shell reads it every frame for drawing and file ops.
type Tabs struct {
	Items   []*Tab
	Active  int
	ScrollX int // horizontal strip scroll (wheel over the tabs, sublime-style)
	counter int // names the next untitled buffer
}

// scrollRange returns how far the strip can scroll (px) before dead
// space appears at the right edge; 0 when every tab fits.
func (t *Tabs) scrollRange(cellW, screenW int) int {
	rects := render.TabBarRects(t.Titles(), cellW)
	if len(rects) == 0 {
		return 0
	}
	last := rects[len(rects)-1]
	total := last.X + last.W + 4
	if r := total - screenW; r > 0 {
		return r
	}
	return 0
}

// ClampScroll keeps ScrollX inside [0, scrollRange].
func (t *Tabs) ClampScroll(cellW, screenW int) {
	if r := t.scrollRange(cellW, screenW); t.ScrollX > r {
		t.ScrollX = r
	}
	if t.ScrollX < 0 {
		t.ScrollX = 0
	}
}

// ScrollBy shifts the strip by dx px (positive reveals later tabs) and
// clamps. Wheel callers pass ebiten deltas inverted.
func (t *Tabs) ScrollBy(dx, cellW, screenW int) {
	t.ScrollX += dx
	t.ClampScroll(cellW, screenW)
}

// Reveal scrolls so tab i is fully visible (called after switches and
// tab creation).
func (t *Tabs) Reveal(i, cellW, screenW int) {
	rects := render.TabBarRects(t.Titles(), cellW)
	if i < 0 || i >= len(rects) {
		return
	}
	r := rects[i]
	if right := r.X + r.W - t.ScrollX; right > screenW {
		t.ScrollX = r.X + r.W - screenW
	}
	if left := r.X - t.ScrollX; left < 8 {
		if t.ScrollX > r.X-8 {
			t.ScrollX = r.X - 8
		}
	}
	t.ClampScroll(cellW, screenW)
}

// Cur returns the active tab (nil when the strip is empty).
func (t *Tabs) Cur() *Tab {
	if t == nil || t.Active < 0 || t.Active >= len(t.Items) {
		return nil
	}
	return t.Items[t.Active]
}

// Titles returns the strip titles in draw/hit-test order.
func (t *Tabs) Titles() []string {
	out := make([]string, len(t.Items))
	for i, tab := range t.Items {
		out[i] = render.TabTitle(tab.Path)
	}
	return out
}

// saveCaret remembers the active tab's caret before the strip changes.
func (e *Editor) saveCaret() {
	t := e.svc.Tabs
	if t == nil {
		return
	}
	if cur := t.Cur(); cur != nil {
		cur.Caret = e.svc.Engine.Caret()
	}
}

// NewTab appends a fresh local buffer and activates it.
func (e *Editor) NewTab() {
	t := e.svc.Tabs
	if t == nil {
		return
	}
	e.saveCaret()
	t.counter++
	path := fmt.Sprintf("untitled-%d", t.counter)
	t.Items = append(t.Items, &Tab{Doc: doc.New(), Path: path})
	t.Active = len(t.Items) - 1
	t.Reveal(t.Active, e.svc.Layout.CellW, e.svc.Layout.ScreenW)
	log.Printf("new tab: %s", path)
	e.applyActive()
	e.svc.Engine.SetStatus("NEW TAB " + path)
}

// CloseTab closes the active tab (refusing to close the last one) and
// activates a neighbour.
func (e *Editor) CloseTab() {
	t := e.svc.Tabs
	if t == nil || len(t.Items) <= 1 {
		e.svc.Engine.SetStatus("CANNOT CLOSE THE LAST TAB")
		return
	}
	e.saveCaret()
	i := t.Active
	closing := t.Items[i]
	if closing.Doc.Dirty() {
		log.Printf("closing dirty tab %q (changes discarded)", closing.Path)
	} else {
		log.Printf("closing tab %q", closing.Path)
	}
	t.Items = append(t.Items[:i], t.Items[i+1:]...)
	if i >= len(t.Items) {
		i = len(t.Items) - 1
	}
	t.Active = i
	t.Reveal(i, e.svc.Layout.CellW, e.svc.Layout.ScreenW)
	e.applyActive()
}

// SwitchTab activates tab i (saving the current caret first).
func (e *Editor) SwitchTab(i int) {
	t := e.svc.Tabs
	if t == nil || i < 0 || i >= len(t.Items) || i == t.Active {
		return
	}
	e.saveCaret()
	t.Active = i
	t.Reveal(i, e.svc.Layout.CellW, e.svc.Layout.ScreenW)
	log.Printf("switch to tab %d: %q", i, render.TabTitle(t.Cur().Path))
	e.applyActive()
}

// NextTab activates the next tab, wrapping to the first (Ctrl+PageDown).
func (e *Editor) NextTab() {
	t := e.svc.Tabs
	if t == nil || len(t.Items) == 0 {
		return
	}
	e.SwitchTab((t.Active + 1) % len(t.Items))
}

// PrevTab activates the previous tab, wrapping to the last (Ctrl+PageUp).
func (e *Editor) PrevTab() {
	t := e.svc.Tabs
	if t == nil || len(t.Items) == 0 {
		return
	}
	e.SwitchTab((t.Active + len(t.Items) - 1) % len(t.Items))
}

// applyActive syncs the editor services and the engine with the active
// tab (callers must saveCaret() before changing strip state).
func (e *Editor) applyActive() {
	t := e.svc.Tabs
	if t == nil {
		return
	}
	cur := t.Cur()
	if cur == nil {
		return
	}
	e.svc.Doc = cur.Doc
	e.svc.Path = cur.Path
	e.svc.Engine.SwapDocument(cur.Doc, cur.Caret)
}

// TabHitTest reports the tab index under a screen click (or none).
func (e *Editor) TabHitTest(x, y int) (int, bool) {
	if e.svc.Tabs == nil {
		return 0, false
	}
	return render.TabHit(e.svc.Tabs.Titles(), e.svc.Layout.CellW, e.svc.Tabs.ScrollX, x, y)
}

// setPath retargets the active tab (File > New) and the services copy.
func (e *Editor) setPath(p string) {
	e.svc.Path = p
	if t := e.svc.Tabs; t != nil {
		if cur := t.Cur(); cur != nil {
			cur.Path = p
		}
	}
}
