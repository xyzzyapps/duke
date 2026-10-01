package render

import "strings"

// Menu-bar geometry and labels. Both the renderer (drawing) and the editor
// (hit-testing) use these values, so they can never drift apart.
//
// Layout: the buttons live inside the top HUD bar; a open menu drops a
// dropdown panel directly beneath it, overlaying the text viewport.

// Rect is a simple integer screen rectangle.
type Rect struct {
	X, Y, W, H int
}

// Contains reports whether the point lies inside the rectangle.
func (r Rect) Contains(x, y int) bool {
	return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H
}

// Menu-bar buttons (inside the 48px top bar). Sizes come from the shared
// GUI face: a glyph advances charW px, so "File" is 4*18 = 72px wide.
var (
	FileBtn     = Rect{X: 8, Y: 4, W: 82, H: 40}
	HelpBtn     = Rect{X: 96, Y: 4, W: 82, H: 40}
	SettingsBtn = Rect{X: 184, Y: 4, W: 154, H: 40}
)

// Menu identifiers.
const (
	MenuNone     = ""
	MenuFile     = "file"
	MenuHelp     = "help"
	MenuSettings = "settings"
)

// Menu item labels, in dropdown order. The editor maps indices to actions.
var (
	FileMenuItems     = []string{"New", "Save", "Close"}
	HelpMenuItems     = []string{"Help Topics", "About"}
	SettingsMenuItems = []string{"Sprite Sheet..."}
)

// DropY is where dropdowns start (right below the top bar).
const DropY = BarH

// MenuLabels returns the open menu's item labels and anchor button.
func MenuLabels(open string) ([]string, Rect) {
	switch open {
	case MenuHelp:
		return HelpMenuItems, HelpBtn
	case MenuSettings:
		return SettingsMenuItems, SettingsBtn
	}
	return FileMenuItems, FileBtn
}

// dropItemH is one dropdown row (one line of the shared GUI face + pad).
const dropItemH = 42

// DropWidth returns the dropdown panel width for a label set.
func DropWidth(labels []string) int {
	w := 0
	for _, l := range labels {
		if n := len(l)*charW + 20; n > w {
			w = n
		}
	}
	if w < 70 {
		w = 70
	}
	return w
}

// charW is the shared GUI face advance (JetBrains Mono at 30px = 18px per
// glyph), rounded up so text never clips a rect sized from it.
const charW = 18

// DropRects returns the hit rectangle of every item in an open dropdown
// anchored at the given button's left edge.
func DropRects(btn Rect, labels []string) []Rect {
	w := DropWidth(labels)
	out := make([]Rect, len(labels))
	for i := range labels {
		out[i] = Rect{X: btn.X, Y: DropY + i*dropItemH, W: w, H: dropItemH}
	}
	return out
}

// MenuState is the live menu-bar interaction state: the editor mutates it
// from input events and the renderer draws it.
type MenuState struct {
	Open      string // MenuNone / MenuFile / MenuHelp
	HoverBtn  int    // 0 = File, 1 = Help, -1 = none
	HoverItem int    // index into the open menu, -1 = none
	CursorX   int
	CursorY   int
}

// Dialog is a modal text panel (Help, About) drawn centred on screen.
type Dialog struct {
	Title string
	Lines []string
}

// HelpLines is the content of the Help dialog.
func HelpLines() []string {
	return []string{
		"DUKE - THE GUNMAN TEXT EDITOR",
		"",
		"type ................ he throws letters into the buffer",
		"backspace ........... first press turns him LEFT, next press swings the KATANA",
		"hold backspace ...... turn once, then the sword flurry",
		"delete .............. first press turns him RIGHT, next press strikes",
		"ctrl + backspace .... SHOTGUN blast: kills the word",
		"ctrl + delete ....... shotgun: kills word + trailing spaces",
		"ctrl + k ............ ROCKET LAUNCHER: kill line (emacs)",
		"ctrl + u ............ rocket: kill back to line start (shell)",
		"arrows .............. first press turns that way, next presses walk him",
		"ctrl + up / down .... grabs the line with his hands and drags it",
		"ctrl + z / ctrl + y . undo / redo",
		"ctrl + s ............ save    ctrl + o: reload",
		"sound button (top bar)  toggles mute",
		"file menu (top bar) . new / save / close    help menu: this + about",
		"F1 / esc ............ open or close this dialog",
	}
}

// dialogGeom is the single source of truth for dialog placement (used by
// both the renderer and the link hit-test, so they can never drift).
func dialogGeom(screenW, screenH int, d Dialog) (x, y, w, h, titleH, lineH, pad int) {
	const (
		pad0    = 16
		lineH0  = 42 // shared face line height + a breath of padding
		titleH0 = 46
	)
	width := len(d.Title)*charW + pad0*2
	for _, l := range d.Lines {
		if n := len(l)*charW + pad0*2; n > width {
			width = n
		}
	}
	if max := screenW - 40; width > max {
		width = max
	}
	height := titleH0 + len(d.Lines)*lineH0 + pad0 + 42 + pad0
	return (screenW - width) / 2, (screenH - height) / 2, width, height, titleH0, lineH0, pad0
}

// DialogLineRect returns the hit rectangle of the i-th dialog line (drawn
// by drawDialog), so the editor can make lines clickable. ok is false when
// the index is out of range.
func DialogLineRect(screenW, screenH int, d Dialog, i int) (Rect, bool) {
	if i < 0 || i >= len(d.Lines) {
		return Rect{}, false
	}
	x, y, _, _, titleH, lineH, pad := dialogGeom(screenW, screenH, d)
	ly := y + titleH + i*lineH
	return Rect{X: x + pad, Y: ly, W: len(d.Lines[i]) * charW, H: lineH}, true
}

// DialogLinkRect returns the hit rectangle of the dialog line containing
// url (drawn by drawDialog). ok is false when the dialog has no such line.
func DialogLinkRect(screenW, screenH int, d Dialog, url string) (Rect, bool) {
	for i, l := range d.Lines {
		if strings.Contains(l, url) {
			if rc, ok := DialogLineRect(screenW, screenH, d, i); ok {
				return rc, true
			}
		}
	}
	return Rect{}, false
}

// TabBarHeight is the strip height between the top bar and the viewport.
const TabBarHeight = 44

// TabInfo is one tab for the HUD.
type TabInfo struct {
	Title  string
	Active bool
}

// TabTitle is the short label of a file path in the tab strip.
func TabTitle(path string) string {
	if path == "" {
		return "untitled"
	}
	base := path
	if i := strings.LastIndexAny(path, "/\\"); i >= 0 {
		base = path[i+1:]
	}
	if len(base) > 18 {
		base = base[:17] + "~"
	}
	return base
}

// TabBarRects lays out the tab strip (y = top bar bottom + padding).
func TabBarRects(titles []string) []Rect {
	out := make([]Rect, len(titles))
	x := 8
	for i, t := range titles {
		w := len(t)*charW + 22
		if w < 72 {
			w = 72
		}
		if w > 150 {
			w = 150
		}
		out[i] = Rect{X: x, Y: BarH + 3, W: w, H: TabBarHeight - 6}
		x += w + 4
	}
	return out
}

// TabHit maps a click on the tab strip to a tab index.
func TabHit(titles []string, x, y int) (int, bool) {
	for i, r := range TabBarRects(titles) {
		if r.Contains(x, y) {
			return i, true
		}
	}
	return 0, false
}

// HoverSound is the MenuState.HoverBtn id for the sound toggle; the sheet
// picker lives in the Settings menu (id 3, alongside the top bar buttons).
const HoverSound = 2

// HoverSettings is the MenuState.HoverBtn id for the Settings button.
const HoverSettings = 3

// SoundBtnRect is the mute toggle in the top bar (right side, before the
// caret position readout). Shared by the renderer and the editor.
func SoundBtnRect(screenW int) Rect {
	return Rect{X: screenW - 480, Y: 4, W: 194, H: 40}
}
