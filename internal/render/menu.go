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

// Menu-bar buttons (inside the 32px top bar).
var (
	FileBtn = Rect{X: 8, Y: 6, W: 34, H: 20}
	HelpBtn = Rect{X: 46, Y: 6, W: 40, H: 20}
)

// Menu identifiers.
const (
	MenuNone = ""
	MenuFile = "file"
	MenuHelp = "help"
)

// Menu item labels, in dropdown order. The editor maps indices to actions.
var (
	FileMenuItems = []string{"New", "Save", "Close"}
	HelpMenuItems = []string{"Help Topics", "About"}
)

// DropY is where dropdowns start (right below the top bar).
const DropY = 32

// itemH is one dropdown row.
const dropItemH = 18

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

// charW is one character at the HUD font scale (1x): advance 6px.
const charW = 6

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
		"backspace ........... giant KATANA swing (pistol when far away)",
		"hold backspace ...... automatic sword flurry",
		"delete / right-click  sword up close, pistol at range",
		"ctrl + backspace .... SHOTGUN blast: kills the word",
		"ctrl + delete ....... shotgun: kills word + trailing spaces",
		"ctrl + k ............ ROCKET LAUNCHER: kill line (emacs)",
		"ctrl + u ............ rocket: kill back to line start (shell)",
		"arrows / click ...... walk there (the caret follows him)",
		"ctrl + up / down .... grabs the line with his hands and drags it",
		"ctrl + z / ctrl + y . undo / redo",
		"ctrl + s ............ save    ctrl + o: reload",
		"file menu (top bar) . new / save / close    help menu: this + about",
		"F1 / esc ............ open or close this dialog",
	}
}

// dialogGeom is the single source of truth for dialog placement (used by
// both the renderer and the link hit-test, so they can never drift).
func dialogGeom(screenW, screenH int, d Dialog) (x, y, w, h, titleH, lineH, pad int) {
	const (
		pad0    = 16
		lineH0  = 18
		titleH0 = 24
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
	height := titleH0 + len(d.Lines)*lineH0 + pad0 + 18 + pad0
	return (screenW - width) / 2, (screenH - height) / 2, width, height, titleH0, lineH0, pad0
}

// DialogLinkRect returns the hit rectangle of the dialog line containing
// url (drawn by drawDialog). ok is false when the dialog has no such line.
func DialogLinkRect(screenW, screenH int, d Dialog, url string) (Rect, bool) {
	for i, l := range d.Lines {
		if strings.Contains(l, url) {
			x, y, _, _, titleH, lineH, pad := dialogGeom(screenW, screenH, d)
			ly := y + titleH + i*lineH
			return Rect{X: x + pad, Y: ly, W: len(l) * charW, H: lineH}, true
		}
	}
	return Rect{}, false
}

// TabBarHeight is the strip height between the top bar and the viewport.
const TabBarHeight = 20

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
