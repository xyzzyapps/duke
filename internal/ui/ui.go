// Package ui maps user input events onto editor commands and owns the
// editor-level concerns: file save/reload and the help overlay.
//
// The editor subscribes to the event bus (published by the platform shell
// in cmd/shooter) and translates semantic events into calls on the action
// engine. It never touches rendering or Ebitengine input directly, which
// keeps the whole interaction model unit-testable with a mock engine.
package ui

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"

	"shooter/internal/actions"
	"shooter/internal/doc"
	"shooter/internal/events"
	"shooter/internal/fileio"
	"shooter/internal/render"
)

// tabWidth is how many spaces a Tab press inserts (the buffer stays
// rune-based, so spaces keep column maths simple).
const tabWidth = 4

// Services is the editor's service locator: every collaborator the UI
// needs, assembled by the shell.
// Messenger sends a chat line to the LAN session (nil in solo play).
type Messenger interface {
	SendChat(text string)
}

type Services struct {
	Bus         events.Bus
	Doc         doc.Document
	Engine      actions.Engine
	Store       fileio.Store
	Layout      *render.Layout
	Path        string                 // file backing this session ("" = nowhere to save)
	Net         Messenger              // LAN session (nil in solo play)
	Multiplayer bool                   // connected: undo/redo disabled
	OpenURL     func(url string) error // browser opener (shell-provided)
	// PickSavePath asks the user where to save an untitled buffer
	// (native file dialog in the shell). Returns ok=false on cancel.
	PickSavePath func(def string) (string, bool)
	Tabs         *Tabs // shared tab strip (nil = single buffer)
}

// GumroadURL is the support link shown in the About dialog and README.
const GumroadURL = "https://xyzzy.gumroad.com/l/ykqqqy"

// Editor translates input events into engine commands and owns the menu
// bar (File/Help), the dialogs and file operations.
type Editor struct {
	svc        Services
	Help       bool // help dialog visible
	About      bool // about dialog visible
	menu       render.MenuState
	closeArmed bool // File > Close needs a second click when dirty
	chatOpen   bool // F2 chat draft line
	chatDraft  string
}

// ChatDraft returns the open chat draft line, or nil when not chatting.
func (e *Editor) ChatDraft() *string {
	if !e.chatOpen {
		return nil
	}
	return &e.chatDraft
}

// MenuState returns the live menu-bar state for the HUD (never nil).
func (e *Editor) MenuState() *render.MenuState { return &e.menu }

// Dialog returns the modal dialog to draw, or nil when none is open.
func (e *Editor) Dialog() *render.Dialog {
	switch {
	case e.Help:
		return &render.Dialog{Title: "Help", Lines: render.HelpLines()}
	case e.About:
		return &render.Dialog{Title: "About DUKE", Lines: aboutLines}
	}
	return nil
}

// aboutLines is the About dialog content (support link included).
var aboutLines = []string{
	"DUKE - the gunman text editor",
	"",
	"Support Xyzzy if you want this to be maintained!",
	GumroadURL,
	"",
	"License: PolyForm Noncommercial License 1.0.0",
	"Copyright 2026 Xyzzy Apps",
}

// New creates the editor and subscribes it to the bus.
func New(svc Services) *Editor {
	e := &Editor{
		svc:  svc,
		menu: render.MenuState{HoverBtn: -1, HoverItem: -1},
	}
	svc.Bus.Subscribe(e.OnEvent)
	return e
}

// Path returns the file this session edits.
func (e *Editor) Path() string { return e.svc.Path }

// OnEvent handles one input event. Exported so tests can feed events
// directly as well as through the bus.
func (e *Editor) OnEvent(ev events.Event) {
	switch v := ev.(type) {
	case events.RuneTyped:
		e.onRune(v)
	case events.KeyPressed:
		e.onKey(v)
	case events.MousePressed:
		e.onMouse(v)
	case events.MouseMoved:
		e.onMove(v)
	}
}

// onRune queues typed text. While the help overlay is open, typing is
// swallowed.
func (e *Editor) onRune(v events.RuneTyped) {
	if e.chatOpen {
		if v.Ch >= 32 && v.Ch != 127 && v.Ch != '\t' {
			e.chatDraft += string(v.Ch)
		}
		return
	}
	if e.Help {
		return
	}
	// Control codes and tab are handled as keys (or not at all); the
	// shell already filters Ctrl chords.
	if v.Ch < 32 || v.Ch == 127 || v.Ch == '\t' {
		return
	}
	e.svc.Engine.TypeRune(v.Ch)
}

// onKey maps key presses (with OS-style repeats from the shell) to editor
// commands.
func (e *Editor) onKey(v events.KeyPressed) {
	// While the chat line is open it owns the keyboard.
	if e.chatOpen {
		switch v.Key {
		case ebiten.KeyEnter:
			e.sendChat()
		case ebiten.KeyEscape:
			e.cancelChat()
		case ebiten.KeyBackspace:
			if n := len(e.chatDraft); n > 0 {
				// Trim one whole rune from the end.
				e.chatDraft = e.chatDraft[:len(e.chatDraft)-1]
				for n > 0 && !utf8.RuneStart(e.chatDraft[len(e.chatDraft)-1]) && len(e.chatDraft) > 0 {
					e.chatDraft = e.chatDraft[:len(e.chatDraft)-1]
					n--
				}
			}
		}
		return
	}
	if v.Key == ebiten.KeyF2 {
		e.openChat()
		return
	}
	// Escape closes the open menu first, then dialogs.
	if v.Key == ebiten.KeyEscape && e.menu.Open != render.MenuNone {
		e.closeMenu()
		return
	}
	// Dialogs swallow everything except closing them.
	if e.Help || e.About {
		if v.Key == ebiten.KeyF1 || v.Key == ebiten.KeyEscape {
			e.Help, e.About = false, false
		}
		return
	}
	if v.Key == ebiten.KeyF1 {
		e.Help = true
		e.About = false
		return
	}
	if v.Ctrl {
		e.onCtrl(v)
		return
	}

	d, eng := e.svc.Doc, e.svc.Engine
	caret := eng.Caret()
	switch v.Key {
	case ebiten.KeyBackspace:
		eng.Shoot(true)
	case ebiten.KeyDelete:
		eng.Shoot(false)
	case ebiten.KeyEnter:
		eng.TypeRune('\n')
	case ebiten.KeyTab:
		for i := 0; i < tabWidth; i++ {
			eng.TypeRune(' ')
		}
	case ebiten.KeyLeft:
		eng.WalkTo(leftTarget(d, caret))
	case ebiten.KeyRight:
		eng.WalkTo(rightTarget(d, caret))
	case ebiten.KeyUp:
		eng.WalkTo(verticalTarget(d, caret, -1))
	case ebiten.KeyDown:
		eng.WalkTo(verticalTarget(d, caret, +1))
	case ebiten.KeyHome:
		eng.WalkTo(doc.Pos{Line: caret.Line, Col: 0})
	case ebiten.KeyEnd:
		eng.WalkTo(doc.Pos{Line: caret.Line, Col: d.RuneCount(caret.Line)})
	case ebiten.KeyPageUp:
		eng.WalkTo(verticalTarget(d, caret, -render.Rows))
	case ebiten.KeyPageDown:
		eng.WalkTo(verticalTarget(d, caret, +render.Rows))
	}
}

// onCtrl maps Ctrl chords.
func (e *Editor) onCtrl(v events.KeyPressed) {
	eng := e.svc.Engine
	switch v.Key {
	case ebiten.KeyN:
		e.newFile()
	case ebiten.KeyT:
		e.NewTab()
	case ebiten.KeyW:
		e.CloseTab()
	case ebiten.KeyS:
		e.save()
	case ebiten.KeyO:
		e.reload()
	case ebiten.KeyBackspace:
		eng.ShootWord(true) // shotgun: kill the word behind
	case ebiten.KeyDelete:
		eng.ShootWord(false) // shotgun: kill the word ahead
	case ebiten.KeyK:
		eng.KillLine(false) // rocket: emacs kill-line
	case ebiten.KeyU:
		eng.KillLine(true) // rocket: shell kill-to-start
	case ebiten.KeyA:
		eng.WalkTo(doc.Pos{Line: eng.Caret().Line, Col: 0}) // emacs bol
	case ebiten.KeyE:
		// emacs eol
		c := eng.Caret()
		eng.WalkTo(doc.Pos{Line: c.Line, Col: e.svc.Doc.RuneCount(c.Line)})
	case ebiten.KeyZ, ebiten.KeyY:
		if e.svc.Multiplayer {
			eng.SetStatus("UNDO IS OFFLINE ONLY")
			return
		}
		if v.Key == ebiten.KeyY || v.Shift {
			eng.Redo()
		} else {
			eng.Undo()
		}
	case ebiten.KeyUp:
		eng.MoveLine(-1)
	case ebiten.KeyDown:
		eng.MoveLine(+1)
	}
}

// onMouse routes clicks: open dialogs first, then the menu bar, then the
// text viewport (left walks, right shoots).
func (e *Editor) onMouse(v events.MousePressed) {
	if v.Button != ebiten.MouseButtonLeft && v.Button != ebiten.MouseButtonRight {
		return
	}
	// A click while chatting cancels the draft (Enter sends it).
	if e.chatOpen {
		if v.Button == ebiten.MouseButtonLeft {
			e.cancelChat()
		}
		return
	}
	// Any click closes an open dialog - except a click on the About
	// support link, which opens the browser instead.
	if e.Help || e.About {
		if v.Button == ebiten.MouseButtonLeft && e.About {
			if dlg := e.Dialog(); dlg != nil {
				if rect, ok := render.DialogLinkRect(
					e.svc.Layout.ScreenW, e.svc.Layout.ScreenH,
					*dlg, GumroadURL); ok && rect.Contains(v.X, v.Y) {
					e.openLink(GumroadURL)
					return
				}
			}
		}
		if v.Button == ebiten.MouseButtonLeft {
			e.Help, e.About = false, false
		}
		return
	}
	// Menu buttons toggle their dropdown.
	if v.Button == ebiten.MouseButtonLeft {
		if render.FileBtn.Contains(v.X, v.Y) {
			e.toggleMenu(render.MenuFile, render.FileBtn)
			return
		}
		if render.HelpBtn.Contains(v.X, v.Y) {
			e.toggleMenu(render.MenuHelp, render.HelpBtn)
			return
		}
		// A click inside the open dropdown runs the item.
		if e.menu.Open != render.MenuNone {
			if e.clickMenuItem(v.X, v.Y) {
				return
			}
			e.closeMenu() // click elsewhere dismisses it
			return
		}
	}
	// Tab strip clicks switch tabs.
	if v.Button == ebiten.MouseButtonLeft {
		if i, ok := e.TabHitTest(v.X, v.Y); ok {
			e.SwitchTab(i)
			return
		}
	}
	// Viewport actions.
	p, ok := e.svc.Layout.ScreenToCell(v.X, v.Y, e.svc.Doc)
	if !ok {
		return
	}
	switch v.Button {
	case ebiten.MouseButtonLeft:
		e.svc.Engine.WalkTo(p)
	case ebiten.MouseButtonRight:
		e.svc.Engine.ShootAt(p)
	}
}

// onMove tracks the cursor for menu hover highlighting.
func (e *Editor) onMove(v events.MouseMoved) {
	e.menu.CursorX, e.menu.CursorY = v.X, v.Y
	e.updateHover()
}

// toggleMenu opens or closes a dropdown.
func (e *Editor) toggleMenu(id string, btn render.Rect) {
	if e.menu.Open == id {
		e.closeMenu()
		return
	}
	e.menu.Open = id
	e.updateHover()
}

// closeMenu dismisses the open dropdown.
func (e *Editor) closeMenu() {
	e.menu.Open = render.MenuNone
	e.menu.HoverItem = -1
}

// menuLabels returns the open menu's labels and anchor button.
func (e *Editor) menuLabels() ([]string, render.Rect) {
	if e.menu.Open == render.MenuHelp {
		return render.HelpMenuItems, render.HelpBtn
	}
	return render.FileMenuItems, render.FileBtn
}

// clickMenuItem runs the hovered/clicked item of the open dropdown.
func (e *Editor) clickMenuItem(x, y int) bool {
	labels, btn := e.menuLabels()
	for i, rc := range render.DropRects(btn, labels) {
		if rc.Contains(x, y) {
			e.closeMenu()
			if e.menu.Open == render.MenuHelp || btn == render.HelpBtn {
				e.helpAction(i)
			} else {
				e.fileAction(i)
			}
			return true
		}
	}
	return false
}

// updateHover recomputes button/item hover from the last cursor position.
func (e *Editor) updateHover() {
	e.menu.HoverBtn, e.menu.HoverItem = -1, -1
	x, y := e.menu.CursorX, e.menu.CursorY
	switch {
	case render.FileBtn.Contains(x, y):
		e.menu.HoverBtn = 0
	case render.HelpBtn.Contains(x, y):
		e.menu.HoverBtn = 1
	}
	if e.menu.Open != render.MenuNone {
		labels, btn := e.menuLabels()
		for i, rc := range render.DropRects(btn, labels) {
			if rc.Contains(x, y) {
				e.menu.HoverItem = i
				break
			}
		}
	}
}

// fileAction implements File > New / Save / Close.
func (e *Editor) fileAction(i int) {
	switch i {
	case 0:
		e.newFile()
	case 1:
		e.save()
	case 2:
		e.close()
	}
}

// helpAction implements Help > Help Topics / About.
func (e *Editor) helpAction(i int) {
	switch i {
	case 0:
		e.About = false
		e.Help = true
	case 1:
		e.Help = false
		e.About = true
	}
}

// newFile clears the active buffer and retargets it at a fresh untitled
// name (file operations reset the armed Close).
func (e *Editor) newFile() {
	log.Printf("new buffer")
	e.svc.Doc.Load("")
	e.svc.Engine.Cancel()
	e.closeArmed = false
	if t := e.svc.Tabs; t != nil {
		t.counter++
		e.setPath(fmt.Sprintf("untitled-%d", t.counter))
	}
	e.svc.Engine.SetStatus("NEW BUFFER")
}

// close implements File > Close: a dirty buffer needs a second click
// (armed) before quitting; then it asks the shell to exit.
func (e *Editor) close() {
	if e.svc.Doc.Dirty() && !e.closeArmed {
		e.closeArmed = true
		e.svc.Engine.SetStatus("UNSAVED CHANGES - CHOOSE CLOSE AGAIN TO DISCARD")
		return
	}
	log.Printf("close requested")
	e.svc.Bus.Publish(events.QuitRequested{})
}

// save writes the ACTIVE TAB to a file. Named buffers save in place;
// untitled buffers first ask where to go (native picker from the shell,
// falling back to the tab name in the working directory).
func (e *Editor) save() {
	path := e.svc.Path
	if path == "" || isUntitled(path) {
		picked, ok := e.pickSavePath(path)
		if !ok {
			e.svc.Engine.SetStatus("SAVE CANCELLED")
			return
		}
		path = picked
		e.setPath(path) // the tab now belongs to that file
	}
	if path == "" {
		e.svc.Engine.SetStatus("NO FILE TO SAVE")
		return
	}
	if err := e.svc.Store.Write(path, e.svc.Doc.Text()); err != nil {
		log.Printf("save failed: %v", err)
		e.svc.Engine.SetStatus(truncate("SAVE FAILED: "+err.Error(), 44))
		return
	}
	log.Printf("saved %s (%d bytes)", path, len(e.svc.Doc.Text()))
	e.svc.Doc.MarkSaved()
	e.closeArmed = false
	e.svc.Engine.SetStatus("SAVED " + filepath.Base(path))
	e.svc.Engine.Celebrate()
}

// isUntitled reports whether a path is a scratch buffer name (untitled,
// untitled-2, ...), i.e. not yet saved to a real file.
func isUntitled(p string) bool {
	base := filepath.Base(p)
	return base == "untitled" || strings.HasPrefix(base, "untitled-")
}

// pickSavePath consults the shell dialog; with no picker available the
// given default is used as-is (working directory fallback).
func (e *Editor) pickSavePath(def string) (string, bool) {
	if e.svc.PickSavePath == nil {
		if def == "" {
			def = "untitled.txt"
		}
		return def, true
	}
	return e.svc.PickSavePath(def)
}

// reload replaces the buffer with the file on disk.
func (e *Editor) reload() {
	e.loadFromDisk("RELOADED ")
}

// LoadFile performs the initial disk read at startup: a missing file
// starts a fresh buffer, read errors fall back to an empty buffer with a
// status message.
func (e *Editor) LoadFile() {
	e.loadFromDisk("")
}

// loadFromDisk reads e.svc.Path into the buffer and resets the gunman.
// prefix labels a successful load ("" for the initial load).
func (e *Editor) loadFromDisk(prefix string) {
	eng := e.svc.Engine
	path := e.svc.Path
	if path == "" {
		eng.SetStatus("NO FILE")
		return
	}
	content, err := e.svc.Store.Read(path)
	switch {
	case err == nil:
		log.Printf("loaded %s (%d bytes)", path, len(content))
		e.svc.Doc.Load(content)
		eng.Cancel()
		if prefix != "" {
			eng.SetStatus(prefix + filepath.Base(path))
		}
	case errors.Is(err, os.ErrNotExist):
		log.Printf("no file at %s, starting a new buffer", path)
		e.svc.Doc.Load("")
		eng.Cancel()
		if prefix != "" {
			eng.SetStatus("NEW FILE " + filepath.Base(path))
		}
	default:
		log.Printf("load failed: %v", err)
		eng.SetStatus(truncate("LOAD FAILED: "+err.Error(), 44))
	}
}

// --- caret target helpers --------------------------------------------------

// leftTarget returns the cell one rune left of from (end of the previous
// line when at column 0).
func leftTarget(d doc.Document, from doc.Pos) doc.Pos {
	if from.Col > 0 {
		return doc.Pos{Line: from.Line, Col: from.Col - 1}
	}
	if from.Line > 0 {
		return doc.Pos{Line: from.Line - 1, Col: d.RuneCount(from.Line - 1)}
	}
	return from
}

// rightTarget returns the cell one rune right of from (start of the next
// line when at the line end).
func rightTarget(d doc.Document, from doc.Pos) doc.Pos {
	if from.Col < d.RuneCount(from.Line) {
		return doc.Pos{Line: from.Line, Col: from.Col + 1}
	}
	if from.Line < d.LineCount()-1 {
		return doc.Pos{Line: from.Line + 1, Col: 0}
	}
	return from
}

// verticalTarget moves by delta lines, clamping the column to the target
// line's length.
func verticalTarget(d doc.Document, from doc.Pos, delta int) doc.Pos {
	target := from.Line + delta
	if target < 0 {
		target = 0
	}
	if target >= d.LineCount() {
		target = d.LineCount() - 1
	}
	col := from.Col
	if n := d.RuneCount(target); col > n {
		col = n
	}
	return doc.Pos{Line: target, Col: col}
}

// truncate shortens s for the status bar.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// openChat enters chat mode (closing menus/dialogs first).
func (e *Editor) openChat() {
	e.Help, e.About = false, false
	e.closeMenu()
	e.chatOpen = true
	e.chatDraft = ""
}

// cancelChat leaves chat mode without sending.
func (e *Editor) cancelChat() {
	e.chatOpen = false
	e.chatDraft = ""
}

// sendChat ships the draft (trimmed) to the LAN session and closes the
// line. Empty drafts just close.
func (e *Editor) sendChat() {
	text := strings.TrimSpace(e.chatDraft)
	e.cancelChat()
	if text == "" {
		return
	}
	if e.svc.Net != nil {
		e.svc.Net.SendChat(text)
	}
}

// openLink launches the support URL in the system browser (no-op when the
// shell did not provide an opener, e.g. in tests).
func (e *Editor) openLink(url string) {
	if e.svc.OpenURL == nil {
		return
	}
	if err := e.svc.OpenURL(url); err != nil {
		log.Printf("open %s: %v", url, err)
		e.svc.Engine.SetStatus("COULD NOT OPEN BROWSER")
		return
	}
	log.Printf("opened %s", url)
}
