package ui

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"shooter/internal/actions"
	"shooter/internal/doc"
	"shooter/internal/events"
	"shooter/internal/fileio"
	"shooter/internal/grid"
	"shooter/internal/render"
)

// mockEngine records every command the editor issues.
type mockEngine struct {
	runes    []rune
	shoots   []bool
	atShoots []doc.Pos // explicit ShootAt targets
	words    []bool    // shotgun word kills (true = behind)
	kills    []bool    // rocket line kills (true = to line start)
	walks    []doc.Pos
	moves    []int
	steps    []int // Step() calls (arrow keys, turn-first)
	undos    int
	redos    int
	wins     int
	cancels  int
	swapped  []doc.Document // SwapDocument calls
	status   string
	caret    doc.Pos
}

func (m *mockEngine) Tick(dt float64) {}
func (m *mockEngine) TypeRune(r rune) { m.runes = append(m.runes, r) }
func (m *mockEngine) Shoot(back bool) { m.shoots = append(m.shoots, back) }
func (m *mockEngine) ShootAt(p doc.Pos) {
	m.atShoots = append(m.atShoots, p)
}
func (m *mockEngine) ShootWord(back bool) { m.words = append(m.words, back) }
func (m *mockEngine) KillLine(back bool)  { m.kills = append(m.kills, back) }
func (m *mockEngine) WalkTo(p doc.Pos) {
	m.caret = p
	m.walks = append(m.walks, p)
}
func (m *mockEngine) Step(dir int)     { m.steps = append(m.steps, dir) }
func (m *mockEngine) MoveLine(dir int) { m.moves = append(m.moves, dir) }
func (m *mockEngine) Undo()            { m.undos++ }
func (m *mockEngine) Redo()            { m.redos++ }
func (m *mockEngine) Celebrate()       { m.wins++ }
func (m *mockEngine) Cancel()          { m.cancels++ }
func (m *mockEngine) SwapDocument(d doc.Document, caret doc.Pos) {
	m.caret = caret
	m.swapped = append(m.swapped, d)
}
func (m *mockEngine) Caret() doc.Pos { return m.caret }
func (m *mockEngine) View() actions.View {
	return actions.View{Caret: m.caret}
}
func (m *mockEngine) Status() string     { return m.status }
func (m *mockEngine) SetStatus(s string) { m.status = s }

// fakeStore is an in-memory fileio.Store.
type fakeStore struct {
	files    map[string]string
	writeErr error
	writes   []string
}

func (f *fakeStore) Read(path string) (string, error) {
	s, ok := f.files[path]
	if !ok {
		return "", &os.PathError{Op: "open", Path: path, Err: os.ErrNotExist}
	}
	return s, nil
}

func (f *fakeStore) Write(path, content string) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	f.files[path] = content
	f.writes = append(f.writes, path)
	return nil
}

// harness wires a real doc/bus/layout with the mock engine and fake store.
type harness struct {
	doc      *doc.Buffer
	engine   *mockEngine
	store    *fakeStore
	bus      events.Bus
	editor   *Editor
	setCalls []string // sheet names passed to SetSheet
}

func newHarness(text string) *harness {
	d := doc.New()
	d.Load(text)
	bus := events.NewBus()
	eng := &mockEngine{}
	store := &fakeStore{files: map[string]string{}}
	layout := render.NewLayout(grid.Grid{CellW: 12, CellH: 32}, 80, 16, 32)
	tabs := &Tabs{Items: []*Tab{{Doc: d, Path: "note.txt"}}}
	h := &harness{doc: d, engine: eng, store: store, bus: bus}
	ed := New(Services{
		Bus:    bus,
		Doc:    d,
		Engine: eng,
		Store:  store,
		Layout: &layout,
		Path:   "note.txt",
		Tabs:   tabs,
		Sheets: func() ([]string, string) { return []string{"duke"}, "duke" },
		SetSheet: func(name string) bool {
			h.setCalls = append(h.setCalls, name)
			return true
		},
	})
	h.editor = ed
	return h
}

// --- tests -----------------------------------------------------------------

func TestRuneTypedQueuesText(t *testing.T) {
	h := newHarness("")
	h.bus.Publish(events.RuneTyped{Ch: 'h'})
	h.bus.Publish(events.RuneTyped{Ch: 'i'})
	if string(h.engine.runes) != "hi" {
		t.Fatalf("runes = %q, want hi", h.engine.runes)
	}
}

func TestControlCodesAndTabAreNotTyped(t *testing.T) {
	h := newHarness("")
	h.bus.Publish(events.RuneTyped{Ch: '\t'})
	h.bus.Publish(events.RuneTyped{Ch: 3}) // Ctrl chord control code
	h.bus.Publish(events.RuneTyped{Ch: 127})
	if len(h.engine.runes) != 0 {
		t.Fatalf("runes = %q, want none", h.engine.runes)
	}
	// Tab as a key inserts spaces.
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyTab})
	if string(h.engine.runes) != "    " {
		t.Fatalf("runes = %q, want 4 spaces", h.engine.runes)
	}
}

func TestEnterTypesNewline(t *testing.T) {
	h := newHarness("")
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyEnter})
	if len(h.engine.runes) != 1 || h.engine.runes[0] != '\n' {
		t.Fatalf("runes = %q, want newline", h.engine.runes)
	}
}

func TestBackspaceAndDeleteShoot(t *testing.T) {
	h := newHarness("")
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyBackspace})
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyDelete})
	if len(h.engine.shoots) != 2 || !h.engine.shoots[0] || h.engine.shoots[1] {
		t.Fatalf("shoots = %v, want [true false]", h.engine.shoots)
	}
}

func TestArrowKeysIssueStep(t *testing.T) {
	h := newHarness("ab\ncdefgh\nxy")
	h.engine.caret = doc.Pos{Line: 1, Col: 2}

	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyLeft})
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyRight})
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyRight})
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyLeft})
	got := h.engine.steps
	if len(got) != 4 || got[0] != -1 || got[1] != 1 || got[2] != 1 || got[3] != -1 {
		t.Fatalf("steps = %v, want [-1 1 1 -1]", got)
	}
	if len(h.engine.walks) != 0 {
		t.Fatalf("arrows must not call WalkTo directly: %v", h.engine.walks)
	}
}

func TestUpDownClampColumn(t *testing.T) {
	h := newHarness("ab\ncdefgh")
	h.engine.caret = doc.Pos{Line: 1, Col: 6}
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyUp})
	if got := h.engine.walks[len(h.engine.walks)-1]; got != (doc.Pos{Line: 0, Col: 2}) {
		t.Fatalf("up clamped -> %+v, want {0 2}", got)
	}
	// At the top of the buffer, up is a no-op target.
	h.engine.caret = doc.Pos{Line: 0, Col: 1}
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyUp})
	if got := h.engine.walks[len(h.engine.walks)-1]; got != (doc.Pos{Line: 0, Col: 1}) {
		t.Fatalf("up at top -> %+v", got)
	}
}

func TestHomeEnd(t *testing.T) {
	h := newHarness("hello")
	h.engine.caret = doc.Pos{Line: 0, Col: 3}
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyHome})
	if got := h.engine.walks[len(h.engine.walks)-1]; got.Col != 0 {
		t.Fatalf("home -> %+v", got)
	}
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyEnd})
	if got := h.engine.walks[len(h.engine.walks)-1]; got.Col != 5 {
		t.Fatalf("end -> %+v", got)
	}
}

func TestCtrlChords(t *testing.T) {
	h := newHarness("")
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyZ, Ctrl: true})
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyZ, Ctrl: true, Shift: true})
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyY, Ctrl: true})
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyUp, Ctrl: true})
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyDown, Ctrl: true})
	if h.engine.undos != 1 {
		t.Fatalf("undos = %d, want 1", h.engine.undos)
	}
	if h.engine.redos != 2 {
		t.Fatalf("redos = %d, want 2 (Ctrl+Shift+Z and Ctrl+Y)", h.engine.redos)
	}
	if len(h.engine.moves) != 2 || h.engine.moves[0] != -1 || h.engine.moves[1] != 1 {
		t.Fatalf("moves = %v, want [-1 1]", h.engine.moves)
	}
}

func TestPlainLettersAreNotChords(t *testing.T) {
	h := newHarness("")
	// 'z' without Ctrl must not undo.
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyZ})
	if h.engine.undos != 0 {
		t.Fatal("plain Z must not undo")
	}
}

func TestCtrlSSavesAndCelebrates(t *testing.T) {
	h := newHarness("content")
	h.doc.Insert(doc.Pos{Line: 0, Col: 7}, "!")
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyS, Ctrl: true})
	if len(h.store.writes) != 1 || h.store.writes[0] != "note.txt" {
		t.Fatalf("writes = %v", h.store.writes)
	}
	if h.store.files["note.txt"] != "content!" {
		t.Fatalf("saved = %q", h.store.files["note.txt"])
	}
	if h.doc.Dirty() {
		t.Fatal("dirty flag must clear after save")
	}
	if h.engine.wins != 1 {
		t.Fatal("save must celebrate")
	}
	if !strings.Contains(h.engine.status, "SAVED") {
		t.Fatalf("status = %q", h.engine.status)
	}
}

func TestCtrlSWriteFailureReportsStatus(t *testing.T) {
	h := newHarness("x")
	h.doc.Insert(doc.Pos{Line: 0, Col: 1}, "!") // make the buffer dirty
	h.store.writeErr = errors.New("disk full")
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyS, Ctrl: true})
	if h.engine.wins != 0 {
		t.Fatal("failed save must not celebrate")
	}
	if !strings.Contains(h.engine.status, "SAVE FAILED") {
		t.Fatalf("status = %q", h.engine.status)
	}
	if !h.doc.Dirty() {
		t.Fatal("failed save must keep the dirty flag")
	}
}

func TestCtrlOReloadsFile(t *testing.T) {
	h := newHarness("old")
	h.store.files["note.txt"] = "fresh from disk"
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyO, Ctrl: true})
	if h.doc.Text() != "fresh from disk" {
		t.Fatalf("text = %q", h.doc.Text())
	}
	if h.engine.cancels != 1 {
		t.Fatal("reload must reset the engine")
	}
	if !strings.Contains(h.engine.status, "RELOADED") {
		t.Fatalf("status = %q", h.engine.status)
	}
}

func TestCtrlOMissingFileStartsEmpty(t *testing.T) {
	h := newHarness("old") // no file in the store
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyO, Ctrl: true})
	if h.doc.Text() != "" {
		t.Fatalf("text = %q, want empty", h.doc.Text())
	}
	if !strings.Contains(h.engine.status, "NEW FILE") {
		t.Fatalf("status = %q", h.engine.status)
	}
}

func TestLoadFileAtStartupToleratesMissingFile(t *testing.T) {
	h := newHarness("preset")
	h.editor.LoadFile() // store has no note.txt
	if h.doc.Text() != "" {
		t.Fatalf("text = %q, want empty", h.doc.Text())
	}
	if h.engine.status != "" {
		t.Fatalf("initial load must be silent, status = %q", h.engine.status)
	}
}

func TestMouseClickWalksToCell(t *testing.T) {
	h := newHarness("line one\nline two")
	// Click on line 1, col 3 inside the viewport (origin from the live
	// layout so tab-strip changes never stale this test).
	lay := h.editor.svc.Layout
	x := lay.OriginX + 3*lay.Grid.CellW + 2
	y := lay.OriginY + 1*lay.Grid.CellH + 5
	h.bus.Publish(events.MousePressed{X: x, Y: y, Button: ebiten.MouseButtonLeft})
	if len(h.engine.walks) != 1 {
		t.Fatalf("walks = %v, want one", h.engine.walks)
	}
	if h.engine.walks[0] != (doc.Pos{Line: 1, Col: 3}) {
		t.Fatalf("walk = %+v, want {1 3}", h.engine.walks[0])
	}
}

func TestClickOnHUDIsIgnored(t *testing.T) {
	h := newHarness("x")
	h.bus.Publish(events.MousePressed{X: 10, Y: 5, Button: ebiten.MouseButtonLeft})
	if len(h.engine.walks) != 0 {
		t.Fatalf("walks = %v, want none", h.engine.walks)
	}
}

func TestRightClickShootsClickedCell(t *testing.T) {
	h := newHarness("line one\nline two")
	// Right-click line 0, col 5: he shoots it from where he stands.
	lay := h.editor.svc.Layout
	x := lay.OriginX + 5*lay.Grid.CellW + 2
	y := lay.OriginY + 0*lay.Grid.CellH + 5
	h.bus.Publish(events.MousePressed{X: x, Y: y, Button: ebiten.MouseButtonRight})
	if len(h.engine.atShoots) != 1 {
		t.Fatalf("atShoots = %v, want one", h.engine.atShoots)
	}
	if h.engine.atShoots[0] != (doc.Pos{Line: 0, Col: 5}) {
		t.Fatalf("target = %+v, want {0 5}", h.engine.atShoots[0])
	}
	if len(h.engine.walks) != 0 {
		t.Fatalf("a right-click must not walk, walks = %v", h.engine.walks)
	}
}

func TestHelpOverlaySwallowsInput(t *testing.T) {
	h := newHarness("")
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyF1})
	if !h.editor.Help {
		t.Fatal("F1 must open help")
	}
	h.bus.Publish(events.RuneTyped{Ch: 'a'})
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyBackspace})
	if len(h.engine.runes) != 0 || len(h.engine.shoots) != 0 {
		t.Fatal("help must swallow typing and shooting")
	}
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyEscape})
	if h.editor.Help {
		t.Fatal("Escape must close help")
	}
	// Input works again.
	h.bus.Publish(events.RuneTyped{Ch: 'a'})
	if len(h.engine.runes) != 1 {
		t.Fatal("input must resume after closing help")
	}
}

func TestPathExposure(t *testing.T) {
	h := newHarness("")
	if h.editor.Path() != "note.txt" {
		t.Fatalf("Path = %q", h.editor.Path())
	}
}

var _ actions.Engine = (*mockEngine)(nil)
var _ fileio.Store = (*fakeStore)(nil)

func TestCtrlBackspaceDeleteFireTheShotgun(t *testing.T) {
	h := newHarness("kill one two")
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyBackspace, Ctrl: true})
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyDelete, Ctrl: true})
	if len(h.engine.words) != 2 || !h.engine.words[0] || h.engine.words[1] {
		t.Fatalf("words = %v, want [true false]", h.engine.words)
	}
	// Plain backspace must stay the katana, not the shotgun.
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyBackspace})
	if len(h.engine.shoots) != 1 || !h.engine.shoots[0] {
		t.Fatalf("shoots = %v, want one plain backspace", h.engine.shoots)
	}
}

func TestCtrlKAndCtrlURocketTheLine(t *testing.T) {
	h := newHarness("rocket line")
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyK, Ctrl: true})
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyU, Ctrl: true})
	if len(h.engine.kills) != 2 || h.engine.kills[0] || !h.engine.kills[1] {
		t.Fatalf("kills = %v, want [false true]", h.engine.kills)
	}
	// Plain K must still type 'k'.
	h.bus.Publish(events.RuneTyped{Ch: 'k'})
	if string(h.engine.runes) != "k" {
		t.Fatalf("runes = %q, want k", h.engine.runes)
	}
}

// --- menus and dialogs -----------------------------------------------------

// clickCenter publishes a left click at a rect's centre.
func clickCenter(h *harness, r render.Rect) {
	h.bus.Publish(events.MousePressed{
		X: r.X + r.W/2, Y: r.Y + r.H/2, Button: ebiten.MouseButtonLeft,
	})
}

func TestFileMenuNewClearsBuffer(t *testing.T) {
	h := newHarness("precious words")
	clickCenter(h, render.FileBtn)
	if h.editor.MenuState().Open != render.MenuFile {
		t.Fatalf("open = %q, want file menu", h.editor.MenuState().Open)
	}
	clickCenter(h, render.DropRects(render.FileBtn, render.FileMenuItems)[0]) // New
	if h.doc.Text() != "" {
		t.Fatalf("text = %q, want empty after New", h.doc.Text())
	}
	if h.editor.MenuState().Open != render.MenuNone {
		t.Fatal("menu must close after picking an item")
	}
	if !strings.Contains(h.engine.status, "NEW BUFFER") {
		t.Fatalf("status = %q", h.engine.status)
	}
}

func TestFileMenuSaveWrites(t *testing.T) {
	h := newHarness("written by mouse")
	clickCenter(h, render.FileBtn)
	clickCenter(h, render.DropRects(render.FileBtn, render.FileMenuItems)[1]) // Save
	if h.store.files["note.txt"] != "written by mouse" {
		t.Fatalf("stored = %q", h.store.files["note.txt"])
	}
	if h.engine.wins != 1 {
		t.Fatal("menu save must celebrate like Ctrl+S")
	}
}

func TestFileMenuClosePublishesQuit(t *testing.T) {
	h := newHarness("clean")
	quit := 0
	h.bus.Subscribe(func(e events.Event) {
		if _, ok := e.(events.QuitRequested); ok {
			quit++
		}
	})
	clickCenter(h, render.FileBtn)
	clickCenter(h, render.DropRects(render.FileBtn, render.FileMenuItems)[2]) // Close
	if quit != 1 {
		t.Fatalf("quit events = %d, want 1", quit)
	}
}

func TestFileMenuCloseConfirmsWhenDirty(t *testing.T) {
	h := newHarness("dirty")
	h.doc.Insert(doc.Pos{Line: 0, Col: 5}, "!")
	quit := 0
	h.bus.Subscribe(func(e events.Event) {
		if _, ok := e.(events.QuitRequested); ok {
			quit++
		}
	})
	clickCenter(h, render.FileBtn)
	clickCenter(h, render.DropRects(render.FileBtn, render.FileMenuItems)[2]) // Close
	if quit != 0 {
		t.Fatal("first close on a dirty buffer must warn, not quit")
	}
	if !strings.Contains(h.engine.status, "UNSAVED CHANGES") {
		t.Fatalf("status = %q", h.engine.status)
	}
	// Second close discards.
	clickCenter(h, render.FileBtn)
	clickCenter(h, render.DropRects(render.FileBtn, render.FileMenuItems)[2])
	if quit != 1 {
		t.Fatalf("quit events = %d, want 1 after confirming", quit)
	}
}

func TestHelpMenuOpensAboutWithSupportLink(t *testing.T) {
	h := newHarness("")
	clickCenter(h, render.HelpBtn)
	if h.editor.MenuState().Open != render.MenuHelp {
		t.Fatalf("open = %q, want help menu", h.editor.MenuState().Open)
	}
	clickCenter(h, render.DropRects(render.HelpBtn, render.HelpMenuItems)[1]) // About
	dlg := h.editor.Dialog()
	if dlg == nil {
		t.Fatal("About must open a dialog")
	}
	joined := strings.Join(dlg.Lines, "\n")
	if !strings.Contains(joined, "https://xyzzy.gumroad.com/l/ykqqqy") {
		t.Fatalf("about dialog missing gumroad link:\n%s", joined)
	}
	if !strings.Contains(joined, "Support Xyzzy if you want this to be maintained!") {
		t.Fatalf("about dialog missing support line:\n%s", joined)
	}
	// A click closes it.
	clickCenter(h, render.Rect{X: 100, Y: 100, W: 1, H: 1})
	if h.editor.Dialog() != nil {
		t.Fatal("click must close the About dialog")
	}
}

func TestHelpMenuOpensHelpDialog(t *testing.T) {
	h := newHarness("")
	clickCenter(h, render.HelpBtn)
	clickCenter(h, render.DropRects(render.HelpBtn, render.HelpMenuItems)[0]) // Help Topics
	dlg := h.editor.Dialog()
	if dlg == nil || dlg.Title != "Help" {
		t.Fatalf("dialog = %+v, want Help", dlg)
	}
	if !strings.Contains(strings.Join(dlg.Lines, "\n"), "KATANA") {
		t.Fatal("help dialog must describe the katana")
	}
	// Escape closes it.
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyEscape})
	if h.editor.Dialog() != nil {
		t.Fatal("Escape must close the dialog")
	}
}

func TestMenuHoverTracking(t *testing.T) {
	h := newHarness("")
	h.bus.Publish(events.MouseMoved{X: 10, Y: 10})
	if h.editor.MenuState().HoverBtn != 0 {
		t.Fatalf("hover = %d, want File button (0)", h.editor.MenuState().HoverBtn)
	}
	h.bus.Publish(events.MouseMoved{X: 60, Y: 400})
	if h.editor.MenuState().HoverBtn != -1 {
		t.Fatalf("hover = %d, want none", h.editor.MenuState().HoverBtn)
	}
}

func TestClickOutsideOpenMenuDismissesIt(t *testing.T) {
	h := newHarness("text")
	clickCenter(h, render.FileBtn)
	if h.editor.MenuState().Open == render.MenuNone {
		t.Fatal("menu should be open")
	}
	clickCenter(h, render.Rect{X: 500, Y: 300, W: 1, H: 1})
	if h.editor.MenuState().Open != render.MenuNone {
		t.Fatal("outside click must dismiss the menu")
	}
	// ...and the click did not leak into the viewport as a walk
	if len(h.engine.walks) != 0 {
		t.Fatalf("walks = %v, want none", h.engine.walks)
	}
}

func TestEscapeClosesOpenMenu(t *testing.T) {
	h := newHarness("")
	clickCenter(h, render.FileBtn)
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyEscape})
	if h.editor.MenuState().Open != render.MenuNone {
		t.Fatal("Escape must close the menu")
	}
}

func TestCtrlNNewBuffer(t *testing.T) {
	h := newHarness("gone")
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyN, Ctrl: true})
	if h.doc.Text() != "" {
		t.Fatalf("text = %q, want empty", h.doc.Text())
	}
}

// --- chat + multiplayer -----------------------------------------------------

// fakeNet records sent chat lines.
type fakeNet struct{ sent []string }

func (f *fakeNet) SendChat(text string) { f.sent = append(f.sent, text) }

func TestF2ChatDraftSendFlow(t *testing.T) {
	h := newHarness("")
	net := &fakeNet{}
	h.editor.svc.Net = net

	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyF2})
	if h.editor.ChatDraft() == nil {
		t.Fatal("F2 must open the chat draft")
	}
	// Runes go to the draft, not the buffer.
	h.bus.Publish(events.RuneTyped{Ch: 'g'})
	h.bus.Publish(events.RuneTyped{Ch: 'o'})
	if len(h.engine.runes) != 0 {
		t.Fatalf("runes leaked into the buffer: %q", h.engine.runes)
	}
	if *h.editor.ChatDraft() != "go" {
		t.Fatalf("draft = %q", *h.editor.ChatDraft())
	}
	// Backspace edits the draft.
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyBackspace})
	if *h.editor.ChatDraft() != "g" {
		t.Fatalf("draft = %q after backspace", *h.editor.ChatDraft())
	}
	h.bus.Publish(events.RuneTyped{Ch: 39}) // apostrophe
	h.bus.Publish(events.RuneTyped{Ch: 't'})
	h.bus.Publish(events.RuneTyped{Ch: 'e'})
	h.bus.Publish(events.RuneTyped{Ch: 'm'})
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyEnter})
	if len(net.sent) != 1 || net.sent[0] != "g'tem" {
		t.Fatalf("sent = %v", net.sent)
	}
	if h.editor.ChatDraft() != nil {
		t.Fatal("Enter must close the draft")
	}
	// Typed text works again after chat closes.
	h.bus.Publish(events.RuneTyped{Ch: 'x'})
	if string(h.engine.runes) != "x" {
		t.Fatalf("runes = %q", h.engine.runes)
	}
}

func TestChatEscapeAndClickCancel(t *testing.T) {
	h := newHarness("")
	net := &fakeNet{}
	h.editor.svc.Net = net

	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyF2})
	h.bus.Publish(events.RuneTyped{Ch: 'n'})
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyEscape})
	if h.editor.ChatDraft() != nil {
		t.Fatal("Escape must close the draft")
	}
	// Reopen, then cancel with a viewport click.
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyF2})
	h.bus.Publish(events.RuneTyped{Ch: 'o'})
	h.bus.Publish(events.MousePressed{X: 200, Y: 200, Button: ebiten.MouseButtonLeft})
	if h.editor.ChatDraft() != nil {
		t.Fatal("click must close the draft")
	}
	if len(net.sent) != 0 {
		t.Fatalf("cancelled draft must not send: %v", net.sent)
	}
	if len(h.engine.walks) != 0 {
		t.Fatal("chat-cancel click must not walk the gunman")
	}
}

func TestChatEnterOnEmptyDraftJustCloses(t *testing.T) {
	h := newHarness("")
	net := &fakeNet{}
	h.editor.svc.Net = net
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyF2})
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyEnter})
	if h.editor.ChatDraft() != nil {
		t.Fatal("Enter on empty draft must close")
	}
	if len(net.sent) != 0 {
		t.Fatalf("sent = %v, want none", net.sent)
	}
}

func TestMultiplayerDisablesUndo(t *testing.T) {
	h := newHarness("shared")
	h.editor.svc.Multiplayer = true
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyZ, Ctrl: true})
	if h.engine.undos != 0 {
		t.Fatal("undo must be refused while connected")
	}
	if !strings.Contains(h.engine.status, "UNDO IS OFFLINE ONLY") {
		t.Fatalf("status = %q", h.engine.status)
	}
	// Redo chord too.
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyY, Ctrl: true})
	if h.engine.redos != 0 {
		t.Fatal("redo must be refused while connected")
	}
}

func TestChatDraftNilWhenClosed(t *testing.T) {
	h := newHarness("")
	if h.editor.ChatDraft() != nil {
		t.Fatal("draft must be nil outside chat mode")
	}
}

func TestAboutLinkClickOpensBrowser(t *testing.T) {
	h := newHarness("")
	opened := []string{}
	h.editor.svc.OpenURL = func(u string) error {
		opened = append(opened, u)
		return nil
	}
	// Open About from the Help menu.
	clickCenter(h, render.HelpBtn)
	clickCenter(h, render.DropRects(render.HelpBtn, render.HelpMenuItems)[1])
	dlg := h.editor.Dialog()
	if dlg == nil {
		t.Fatal("About dialog missing")
	}
	lay := h.editor.svc.Layout
	rect, ok := render.DialogLinkRect(lay.ScreenW, lay.ScreenH, *dlg, GumroadURL)
	if !ok {
		t.Fatal("About dialog has no gumroad link line")
	}
	// Click the link: browser opens, dialog stays open.
	h.bus.Publish(events.MousePressed{
		X: rect.X + rect.W/2, Y: rect.Y + rect.H/2,
		Button: ebiten.MouseButtonLeft,
	})
	if len(opened) != 1 || opened[0] != GumroadURL {
		t.Fatalf("opened = %v, want [%s]", opened, GumroadURL)
	}
	if h.editor.Dialog() == nil {
		t.Fatal("clicking the link must keep the dialog open")
	}
	// Clicking any other dialog spot closes it.
	h.bus.Publish(events.MousePressed{
		X: rect.X - 10, Y: rect.Y - 10, Button: ebiten.MouseButtonLeft,
	})
	if h.editor.Dialog() != nil {
		t.Fatal("non-link click must close the dialog")
	}
}

func TestAboutLinkWithoutOpenerIsSafe(t *testing.T) {
	h := newHarness("")
	h.editor.About = true
	dlg := h.editor.Dialog()
	lay := h.editor.svc.Layout
	rect, _ := render.DialogLinkRect(lay.ScreenW, lay.ScreenH, *dlg, GumroadURL)
	h.bus.Publish(events.MousePressed{
		X: rect.X + 1, Y: rect.Y + 1, Button: ebiten.MouseButtonLeft,
	})
	if h.editor.Dialog() == nil {
		t.Fatal("nil opener must not crash; dialog stays open")
	}
	if h.engine.status != "" {
		t.Fatalf("status = %q, want none", h.engine.status)
	}
}

// --- tabs -------------------------------------------------------------------

func TestCtrlTCreatesTabAndCtrlWClosesIt(t *testing.T) {
	h := newHarness("tab zero")
	if len(h.editor.svc.Tabs.Items) != 1 {
		t.Fatal("harness must start with one tab")
	}
	// Remember where tab 0's gunman stands, then switch away.
	h.engine.caret = doc.Pos{Line: 0, Col: 4}
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyT, Ctrl: true})
	if len(h.editor.svc.Tabs.Items) != 2 {
		t.Fatalf("tabs = %d, want 2", len(h.editor.svc.Tabs.Items))
	}
	if h.editor.svc.Tabs.Active != 1 {
		t.Fatalf("active = %d, want the new tab (1)", h.editor.svc.Tabs.Active)
	}
	if h.editor.svc.Path == "note.txt" {
		t.Fatalf("path = %q, want a fresh untitled name", h.editor.svc.Path)
	}
	if len(h.engine.swapped) == 0 {
		t.Fatal("switching must retarget the engine")
	}
	// The old tab keeps its own document and remembered caret.
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyW, Ctrl: true})
	if len(h.editor.svc.Tabs.Items) != 1 || h.editor.svc.Tabs.Active != 0 {
		t.Fatalf("tabs=%d active=%d after close",
			len(h.editor.svc.Tabs.Items), h.editor.svc.Tabs.Active)
	}
	if h.editor.svc.Tabs.Items[0].Caret != (doc.Pos{Line: 0, Col: 4}) {
		t.Fatalf("caret not remembered: %+v", h.editor.svc.Tabs.Items[0].Caret)
	}
	if h.editor.svc.Path != "note.txt" {
		t.Fatalf("path = %q, want note.txt back", h.editor.svc.Path)
	}
}

func TestCannotCloseLastTab(t *testing.T) {
	h := newHarness("only")
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyW, Ctrl: true})
	if len(h.editor.svc.Tabs.Items) != 1 {
		t.Fatal("the last tab must survive")
	}
	if !strings.Contains(h.engine.status, "LAST TAB") {
		t.Fatalf("status = %q", h.engine.status)
	}
}

func TestTabStripClickSwitches(t *testing.T) {
	h := newHarness("zero")
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyT, Ctrl: true}) // now on tab 1
	rects := render.TabBarRects(h.editor.svc.Tabs.Titles())
	// Click tab 0's rect: back to the first tab.
	h.bus.Publish(events.MousePressed{
		X: rects[0].X + 5, Y: rects[0].Y + 3, Button: ebiten.MouseButtonLeft,
	})
	if h.editor.svc.Tabs.Active != 0 {
		t.Fatalf("active = %d, want 0", h.editor.svc.Tabs.Active)
	}
	if h.editor.svc.Path != "note.txt" {
		t.Fatalf("path = %q, want note.txt", h.editor.svc.Path)
	}
	// Viewport was NOT walked by the strip click.
	walksBefore := len(h.engine.walks)
	rects = render.TabBarRects(h.editor.svc.Tabs.Titles())
	h.bus.Publish(events.MousePressed{
		X: rects[1].X + 5, Y: rects[1].Y + 3, Button: ebiten.MouseButtonLeft,
	})
	if len(h.engine.walks) != walksBefore {
		t.Fatal("tab clicks must not leak into the viewport")
	}
}

func TestNewBufferRetargetsActiveTabPath(t *testing.T) {
	h := newHarness("precious")
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyN, Ctrl: true})
	cur := h.editor.svc.Tabs.Cur()
	if cur.Path == "precious" {
		t.Fatalf("tab path = %q, want a fresh untitled name", cur.Path)
	}
	if h.editor.svc.Path != cur.Path {
		t.Fatalf("services path %q != tab path %q", h.editor.svc.Path, cur.Path)
	}
}

// --- save-to-a-file ---------------------------------------------------------

func TestSaveUntitledTabPicksPathAndRetargetsTab(t *testing.T) {
	h := newHarness("scratch")
	h.editor.setPath("untitled-1") // as Ctrl+T leaves it
	picks := 0
	h.editor.svc.PickSavePath = func(def string) (string, bool) {
		picks++
		if def != "untitled-1" {
			t.Fatalf("picker default = %q, want untitled-1", def)
		}
		return "C:/notes/brilliant.txt", true
	}
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyS, Ctrl: true})
	if picks != 1 {
		t.Fatalf("picks = %d, want 1 (untitled must ask)", picks)
	}
	if h.store.files["C:/notes/brilliant.txt"] == "" {
		t.Fatalf("stored = %v", h.store.files)
	}
	// The TAB now belongs to the chosen file.
	if h.editor.svc.Tabs.Cur().Path != "C:/notes/brilliant.txt" {
		t.Fatalf("tab path = %q", h.editor.svc.Tabs.Cur().Path)
	}
	if !strings.Contains(h.engine.status, "brilliant.txt") {
		t.Fatalf("status = %q", h.engine.status)
	}
	// Saving again goes straight through (no second dialog).
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyS, Ctrl: true})
	if picks != 1 {
		t.Fatalf("picks = %d, want still 1 (now a real path)", picks)
	}
}

func TestSaveNamedTabNeverOpensPicker(t *testing.T) {
	h := newHarness("already named")
	picks := 0
	h.editor.svc.PickSavePath = func(string) (string, bool) {
		picks++
		return "", false
	}
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyS, Ctrl: true}) // via menu path too
	if picks != 0 {
		t.Fatalf("picks = %d, want 0 for a named buffer", picks)
	}
	if h.store.files["note.txt"] != "already named" {
		t.Fatalf("stored = %v", h.store.files)
	}
}

func TestSaveCancelKeepsBufferDirty(t *testing.T) {
	h := newHarness("precious")
	h.editor.setPath("untitled-7")
	h.doc.Insert(doc.Pos{Line: 0, Col: 8}, "!")
	h.editor.svc.PickSavePath = func(string) (string, bool) { return "", false }
	h.bus.Publish(events.KeyPressed{Key: ebiten.KeyS, Ctrl: true})
	if len(h.store.writes) != 0 {
		t.Fatalf("writes = %v, want none after cancel", h.store.writes)
	}
	if !strings.Contains(h.engine.status, "CANCELLED") {
		t.Fatalf("status = %q", h.engine.status)
	}
	if !h.doc.Dirty() {
		t.Fatal("cancelled save must keep the buffer dirty")
	}
	if h.engine.wins != 0 {
		t.Fatal("cancelled save must not celebrate")
	}
}

func TestIsUntitledNames(t *testing.T) {
	for _, p := range []string{"untitled", "untitled-2", "/x/untitled-12"} {
		if !isUntitled(p) {
			t.Fatalf("%q should be untitled", p)
		}
	}
	for _, p := range []string{"notes.txt", "a/b/hello.txt", "untitled.txt.bak"} {
		if isUntitled(p) {
			t.Fatalf("%q should NOT be untitled", p)
		}
	}
}

// --- settings / sprite sheets ----------------------------------------------

func TestSettingsMenuOpensSheetDialog(t *testing.T) {
	h := newHarness("hi")
	clickCenter(h, render.SettingsBtn)
	if got := h.editor.MenuState().Open; got != render.MenuSettings {
		t.Fatalf("open = %q, want settings menu", got)
	}
	clickCenter(h, render.DropRects(render.SettingsBtn, render.SettingsMenuItems)[0])
	if !h.editor.Settings {
		t.Fatal("settings dialog should be open")
	}
	dlg := h.editor.Dialog()
	if dlg == nil || dlg.Title != "Settings" {
		t.Fatalf("dialog = %+v, want Settings", dlg)
	}
	joined := strings.Join(dlg.Lines, "|")
	if !strings.Contains(joined, "duke") {
		t.Fatalf("dialog lines = %q, want duke", joined)
	}
	if !strings.Contains(dlg.Lines[0], "*") {
		t.Fatalf("active sheet not marked: %q", dlg.Lines[0])
	}
}

func TestSettingsClickSwitchesSheet(t *testing.T) {
	h := newHarness("hi")
	clickCenter(h, render.SettingsBtn)
	clickCenter(h, render.DropRects(render.SettingsBtn, render.SettingsMenuItems)[0])
	dlg := h.editor.Dialog()
	if dlg == nil {
		t.Fatal("dialog missing")
	}
	// Click the duke line (index 0).
	rc, ok := render.DialogLineRect(h.editor.svc.Layout.ScreenW, h.editor.svc.Layout.ScreenH, *dlg, 0)
	if !ok {
		t.Fatal("no line rect for duke")
	}
	clickCenter(h, rc)
	if len(h.setCalls) != 1 || h.setCalls[0] != "duke" {
		t.Fatalf("setCalls = %v, want [duke]", h.setCalls)
	}
	if h.editor.Settings {
		t.Fatal("dialog should close after picking a sheet")
	}
	if !strings.Contains(h.engine.Status(), "SPRITE SHEET") {
		t.Fatalf("status = %q, want sprite sheet notice", h.engine.Status())
	}
}
