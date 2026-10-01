package render

import (
	"bytes"
	"fmt"
	"image/color"
	"math"
	"sort"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"shooter/internal/fonts"

	"shooter/internal/actions"
	"shooter/internal/agent"
	"shooter/internal/doc"
	"shooter/internal/grid"
)

// Screen geometry (see package docs for how the numbers relate).
const (
	Scale = 2  // art-pixel scale: the 18x24 sprite becomes 36x48 screen px
	Cols  = 80 // text viewport columns (80x20 grid keeps one aspect)
	Rows  = 20 // text viewport rows
	BarH  = 48 // height of each HUD bar (one line of the shared GUI face)
)

// FontSizes are the selectable document/GUI font sizes (Settings dialog).
// The chrome is sized for the 30px face, so every offered size is <= 30px.
var FontSizes = []int{18, 24, 30}

// DefaultFontSize is the face used at launch (matches the original look).
const DefaultFontSize = 30

// Palette: a dark DOS-terminal look.
var (
	colBG     = color.RGBA{13, 17, 23, 255}
	colLine   = color.RGBA{22, 29, 39, 255} // caret row highlight
	colText   = color.RGBA{201, 209, 217, 255}
	colDim    = color.RGBA{110, 118, 129, 255}
	colBar    = color.RGBA{22, 27, 34, 255}
	colBorder = color.RGBA{48, 54, 61, 255}
	colStatus = color.RGBA{227, 179, 65, 255}
	colTitle  = color.RGBA{88, 166, 255, 255}
	colTracer = color.RGBA{255, 214, 106, 255}
	colCaret  = color.RGBA{120, 200, 255, 255}
	// katana
	colHandle    = color.RGBA{40, 34, 30, 255}
	colBlade     = color.RGBA{214, 220, 232, 255}
	colBladeEdge = color.RGBA{255, 255, 255, 255}
	// rocket
	colRocketBody  = color.RGBA{78, 82, 94, 255}
	colRocketFlame = color.RGBA{255, 140, 48, 255}
	// menus + dialogs
	colMenuSel   = color.RGBA{38, 52, 74, 255}
	colActorName = color.RGBA{174, 196, 224, 255}
	colTitleBar  = color.RGBA{44, 78, 120, 255}
	colStrip     = color.RGBA{17, 21, 27, 255}
	colBarText   = color.RGBA{235, 240, 246, 255}
	// scrollbar overlays
	colScrollTrack = color.RGBA{255, 255, 255, 26}
	colScrollThumb = color.RGBA{255, 255, 255, 92}
)

// ScrollbarW is the overlay scrollbar thickness in screen pixels.
const ScrollbarW = 10

// HUD carries the chrome information the renderer needs each frame.
type HUD struct {
	SoundOn bool       // sound toggle state (top bar)
	Menu    *MenuState // menu-bar interaction state (nil = no menu drawn)
	Dialog  *Dialog    // modal dialog (Help/About), nil when closed

	// Multiplayer presentation: remote participants (drawn as extra
	// gunmen with name tags and cartoon chat bubbles) and the local
	// player's own bubble / chat draft line.
	Actors    []Actor
	LocalChat *Bubble
	ChatDraft *string   // open chat input (drawn in the bottom bar)
	Tabs      []TabInfo // tab strip (drawn below the top bar)
	TabScroll int       // strip scroll in px (Tabs.ScrollX; wheel-scrolled)
}

// Actor is one remote participant's presentation state (world pixels).
type Actor struct {
	X, Y   float64
	Facing int
	State  agent.State
	T      float64 // animation clock (breathing/walk phase)
	Name   string  // tag above his head ("" = no tag)
	Chat   string  // current chat bubble text ("" = none)
	ChatT  float64 // seconds since Chat was set (drives the fade)
}

// Bubble is a chat balloon: text plus its age in seconds.
type Bubble struct {
	Text string
	T    float64
}

// Renderer owns the layout, sprite sheet and particle system.
type Renderer struct {
	bufFace   text.Face               // one face for everything: document + chrome
	bufLineH  int                     // ceil of the face line height (px)
	fontSize  int                     // active face size (see FontSizes)
	sheets    map[string]*spriteSheet // selectable character sheets
	active    string                  // current sheet name (see SheetNames)
	layout    Layout
	particles []particle
}

// New builds the renderer from the embedded font and the sprite sheets
// under sprites/ (see newFromSheetDir).
func New() (*Renderer, error) {
	return newFromSheetDir(diskSheetDir)
}

// newFromSheetDir is New with an explicit sheet folder (tests point it
// at temp dirs; the app always uses sprites/).
func newFromSheetDir(base string) (*Renderer, error) {
	src, err := text.NewGoTextFaceSource(bytes.NewReader(fonts.JetBrainsMonoTTF))
	if err != nil {
		return nil, err
	}
	// One face everywhere: document, HUD bars, menus, dialogs and tabs
	// all share the same size (the user asked for uniform GUI type).
	size := DefaultFontSize
	bufFace := &text.GoTextFace{Source: src, Size: float64(size)}
	bufM := bufFace.Metrics()
	// Sheets come from <base>/<name>/ pose PNGs (the shipped spaceman
	// art lives there; the old built-in ASCII duke is gone).
	sheets, err := loadDiskSheetsFrom(base)
	if err != nil {
		return nil, err
	}
	if len(sheets) == 0 {
		return nil, fmt.Errorf("no sprite sheets: add pose PNGs under sprites/<name>/")
	}
	active := defaultSheet
	if _, ok := sheets[active]; !ok {
		for name := range sheets { // first folder becomes the default
			active = name
			break
		}
	}
	r := &Renderer{
		fontSize: size,
		bufFace:  bufFace,
		bufLineH: int(math.Ceil(bufM.HAscent + bufM.HDescent)),
		sheets:   sheets,
		active:   active,
	}
	r.rebuildLayout()
	return r, nil
}

// SetFontSize switches the single GUI/document face (Settings > Font
// Size). Only the sizes in FontSizes are accepted. The layout is rebuilt
// so the window still shows Cols x Rows cells; the engine grid must be
// refreshed from Layout().Grid afterwards (the shell does that).
func (r *Renderer) SetFontSize(size int) bool {
	ok := false
	for _, s := range FontSizes {
		if s == size {
			ok = true
			break
		}
	}
	if !ok {
		return false
	}
	if size == r.fontSize {
		return true // already active: nothing to do
	}
	src, err := text.NewGoTextFaceSource(bytes.NewReader(fonts.JetBrainsMonoTTF))
	if err != nil {
		return false
	}
	r.fontSize = size
	r.bufFace = &text.GoTextFace{Source: src, Size: float64(size)}
	bufM := r.bufFace.Metrics()
	r.bufLineH = int(math.Ceil(bufM.HAscent + bufM.HDescent))
	r.rebuildLayout()
	return true
}

// FontSizeList returns the selectable sizes and the active one.
func (r *Renderer) FontSizeList() ([]int, int) {
	return FontSizes, r.fontSize
}

// FontSize returns the active face size.
func (r *Renderer) FontSize() int { return r.fontSize }

// rebuildLayout recreates the camera layout from the current face size.
// Rows are art-pixel locked (the sprite fills one row): cellH never
// changes; cellW follows the face advance so the window keeps showing
// exactly Cols columns.
func (r *Renderer) rebuildLayout() {
	cellW := int(math.Round(text.Advance("M", r.bufFace)))
	if cellW <= 0 {
		cellW = artW / 2 * Scale
	}
	r.layout = NewLayout(grid.Grid{CellW: cellW, CellH: artH * Scale}, Cols, Rows, BarH)
}

// Layout returns the live layout (camera included) for click mapping.
func (r *Renderer) Layout() *Layout { return &r.layout }

// defaultSheet is the character shown at launch.
const defaultSheet = "duke"

// SheetNames lists the selectable character sheets.
func (r *Renderer) SheetNames() []string {
	out := make([]string, 0, len(r.sheets))
	for n := range r.sheets {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// ActiveSheet is the sheet currently used to draw the gunman.
func (r *Renderer) ActiveSheet() string { return r.active }

// SetSheet switches the character sheet. Returns false when the name is
// unknown (the active sheet is left untouched).
func (r *Renderer) SetSheet(name string) bool {
	if _, ok := r.sheets[name]; !ok {
		return false
	}
	r.active = name
	return true
}

// sheet returns the active sprite sheet (never nil while a set exists).
func (r *Renderer) sheet() *spriteSheet { return r.sheets[r.active] }

// ScreenSize returns the fixed window size for Ebitengine's Layout hook.
func (r *Renderer) ScreenSize() (int, int) { return r.layout.ScreenW, r.layout.ScreenH }

// Update advances the camera and the particle system. Pass the same view
// snapshot that Draw receives (its FX were drained when the view was
// created).
func (r *Renderer) Update(dt float64, d doc.Document, v actions.View) {
	cx := v.Agent.X + agent.SpriteW/2
	cy := v.Agent.Y + agent.SpriteH/2
	r.layout.follow(cx, cy, d, dt)
	for _, e := range v.FX {
		spawnParticles(&r.particles, e)
	}
	updateParticles(&r.particles, dt)
}

// Draw renders one frame: background, caret row, buffer text (with line
// drag offsets), the gunman, projectiles, particles, then the HUD chrome.
func (r *Renderer) Draw(screen *ebiten.Image, d doc.Document, v actions.View, hud HUD) {
	l := &r.layout
	screen.Fill(colBG)

	// Caret row highlight (the row the gunman stands on).
	hlY := float64(l.OriginY) + float64(v.Caret.Line*l.CellH) - l.ScrollY
	if hlY < float64(l.OriginY+l.ViewH) && hlY+float64(l.CellH) > float64(l.OriginY) {
		vector.DrawFilledRect(screen, float32(l.OriginX), float32(hlY),
			float32(l.ViewW), float32(l.CellH), colLine, false)
	}

	// Buffer text, one line at a time, with line-swap animation offsets.
	first := int(math.Floor(l.ScrollY / float64(l.CellH)))
	last := int(math.Floor((l.ScrollY + float64(l.ViewH)) / float64(l.CellH)))
	for line := first; line <= last; line++ {
		if line < 0 || line >= d.LineCount() {
			continue
		}
		y := float64(l.OriginY) + float64(line*l.CellH) - l.ScrollY +
			swapOffset(line, v.Swaps, l.CellH)
		// Glyphs are shorter than the cell, so centre each line vertically.
		y += float64((l.CellH - r.bufLineH) / 2)
		r.drawDocLine(screen, d.Line(line), float64(l.OriginX)-l.ScrollX, y, colText)
	}

	// World entities, converted to screen space (the gunman covers his
	// own cell, bullets/particles fly above the text).
	offX := float64(l.OriginX) - l.ScrollX
	offY := float64(l.OriginY) - l.ScrollY
	r.drawAgent(screen, v.Agent, offX, offY)
	r.drawKatana(screen, v.Agent, offX, offY)
	for _, b := range v.Bullets {
		r.drawBullet(screen, b, offX, offY)
	}
	drawParticles(screen, r.particles, offX, offY, r.runeDrawer(screen, offX, offY))

	// Remote participants: gunman + name tag + chat bubble, then the
	// local player's own bubble (his sprite is already drawn above).
	for _, a := range hud.Actors {
		r.drawActor(screen, a, offX, offY)
	}
	if hud.LocalChat != nil && hud.LocalChat.Text != "" {
		cx := v.Agent.X + agent.SpriteW/2 + offX
		r.drawBubble(screen, cx, v.Agent.Y+offY-4, *hud.LocalChat)
	}

	// HUD chrome on top so bleed-over from the text is covered.
	r.drawHUD(screen, v, hud)
	r.drawMenu(screen, hud)
	if hud.Dialog != nil {
		r.drawDialog(screen, *hud.Dialog)
	}
	r.drawScrollbars(screen, d)
}

// swapOffset returns the vertical pixel offset for a row that is taking
// part in a line-drag animation (see actions.Swap).
func swapOffset(line int, swaps []actions.Swap, cellH int) float64 {
	for _, s := range swaps {
		if s.T >= 1 {
			continue
		}
		dy := float64(cellH) * float64(s.Dir) * (1 - s.T)
		switch line {
		case s.Row:
			return dy // displaced line starts one row away
		case s.Row + s.Dir:
			return -dy // the dragged line starts at its old row
		}
	}
	return 0
}

// drawAgent draws the gunman sprite with facing and pose offsets.
// GeoM operations apply in call order, so the chain is: art-space pose
// offset, optional mirror (still in art space), the art->screen scale that
// matches the font's pixel size, then placement in screen space.
func (r *Renderer) drawAgent(screen *ebiten.Image, a agent.Snapshot, offX, offY float64) {
	id, xOff, yOff := r.sheet().pose(a)
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(xOff), float64(yOff))
	if a.Facing > 0 {
		// The pixel array is the LEFT-facing base art; facing right is
		// derived by mirroring around the sprite box.
		op.GeoM.Scale(-1, 1)
		op.GeoM.Translate(artW, 0)
	}
	op.GeoM.Scale(float64(Scale), float64(Scale))
	op.GeoM.Translate(a.X+offX, a.Y+offY)
	op.Filter = ebiten.FilterNearest
	screen.DrawImage(r.sheet().image(id), op)
}

// drawKatana renders the giant blade during a slash: it grows out of his
// hands and sweeps from raised-behind to forward-follow-through, leaving a
// pale arc trail. Everything is vector geometry (the swing is far bigger
// than any sprite frame could be).
func (r *Renderer) drawKatana(screen *ebiten.Image, a agent.Snapshot, offX, offY float64) {
	if a.State != agent.StateSlash {
		return
	}
	const (
		bladeLen = 15 * Scale // giant: about one sprite-height of blade
		handle   = 3 * Scale
	)
	ang := katanaAngle(a.T)
	dir := float64(a.Facing) // blade follows the struck direction
	hx := katanaPivotX(a, offX)
	hy := a.Y + offY + agent.SpriteH/2 - 2*float64(Scale)

	// Trail: an arc the blade just swept through (only while it moves).
	if a.T > 0.06 && a.T < 0.26 {
		prev := katanaAngle(a.T - 0.06)
		const segs = 6
		for i := 0; i < segs; i++ {
			a0 := prev + (ang-prev)*float64(i)/segs
			a1 := prev + (ang-prev)*float64(i+1)/segs
			r0, r1 := bladeLen*0.55, bladeLen*0.9
			fade := float32(120 - i*18)
			if fade < 20 {
				fade = 20
			}
			vector.StrokeLine(screen,
				float32(hx+dir*r0*math.Cos(a0)), float32(hy+r0*math.Sin(a0)),
				float32(hx+dir*r1*math.Cos(a1)), float32(hy+r1*math.Sin(a1)),
				float32(7-i), color.RGBA{255, 255, 255, uint8(fade)}, true)
		}
	}

	// Handle + guard.
	gx := hx + dir*handle*math.Cos(ang)
	gy := hy + handle*math.Sin(ang)
	vector.StrokeLine(screen, float32(hx), float32(hy), float32(gx), float32(gy),
		4, colHandle, true)
	// Blade.
	tx := hx + dir*bladeLen*math.Cos(ang)
	ty := hy + bladeLen*math.Sin(ang)
	vector.StrokeLine(screen, float32(gx), float32(gy), float32(tx), float32(ty),
		3, colBlade, true)
	vector.StrokeLine(screen, float32(gx), float32(gy), float32(tx), float32(ty),
		1, colBladeEdge, true)
}

// katanaAngle maps a swing's elapsed time (seconds) to the blade angle.
// 0..0.10 raises the sword from the sheath, 0.10..0.20 is the cut, and the
// blade holds pointing at the struck glyph at face/eye height afterwards
// (slightly up, so the tip ends near his eyes, not the floor).
func katanaAngle(t float64) float64 {
	const (
		raised = -100 * math.Pi / 180
		cut0   = -40 * math.Pi / 180
		cut1   = -15 * math.Pi / 180
		follow = -15 * math.Pi / 180
	)
	lerp := func(a, b, k float64) float64 { return a + (b-a)*math.Min(1, math.Max(0, k)) }
	switch {
	case t < 0.10:
		return lerp(raised, cut0, t/0.10)
	case t < 0.20:
		return lerp(cut0, cut1, (t-0.10)/0.10)
	default:
		return lerp(cut1, follow, (t-0.20)/0.10)
	}
}

// drawBullet draws the projectile: a tracer for the pistol, a fan of
// pellets for the shotgun, a rocket for the launcher.
func (r *Renderer) drawBullet(screen *ebiten.Image, b actions.Bullet, offX, offY float64) {
	t := math.Min(1, math.Max(0, b.T))
	hx := b.FromX + (b.ToX-b.FromX)*t + offX
	hy := b.FromY + (b.ToY-b.FromY)*t + offY
	tt := math.Max(0, t-0.4)
	tx := b.FromX + (b.ToX-b.FromX)*tt + offX
	ty := b.FromY + (b.ToY-b.FromY)*tt + offY

	if b.Rocket {
		r.drawRocket(screen, tx, ty, hx, hy, t)
		return
	}
	vector.StrokeLine(screen, float32(tx), float32(ty), float32(hx), float32(hy),
		2, colTracer, true)
	if b.Spread {
		// Two extra pellets fanning out around the main tracer.
		for _, deg := range []float64{-9, 9} {
			a := deg * math.Pi / 180
			dx, dy := hx-tx, hy-ty
			rx := dx*math.Cos(a) - dy*math.Sin(a)
			ry := dx*math.Sin(a) + dy*math.Cos(a)
			vector.StrokeLine(screen, float32(tx), float32(ty),
				float32(tx+rx*0.9), float32(ty+ry*0.9), 1, colTracer, true)
		}
	}
	vector.DrawFilledCircle(screen, float32(hx), float32(hy), 2.5, colWhite, true)
}

// drawRocket renders a small rocket with a flame trail from tail to head.
func (r *Renderer) drawRocket(screen *ebiten.Image, tailX, tailY, headX, headY, t float64) {
	dx, dy := headX-tailX, headY-tailY
	len := math.Hypot(dx, dy)
	if len < 0.001 {
		return
	}
	ux, uy := dx/len, dy/len
	// Body: thick dark line with a steel nose.
	vector.StrokeLine(screen, float32(tailX), float32(tailY), float32(headX), float32(headY),
		5, colRocketBody, true)
	vector.DrawFilledCircle(screen, float32(headX), float32(headY), 3, colTracer, true)
	// Flame behind the tail (flickers with the flight time).
	flame := 5 + 3*math.Sin(t*40)
	fx0 := tailX - ux*2
	fy0 := tailY - uy*2
	vector.StrokeLine(screen, float32(fx0), float32(fy0),
		float32(fx0-ux*flame), float32(fy0-uy*flame), 4, colRocketFlame, true)
	vector.StrokeLine(screen, float32(fx0), float32(fy0),
		float32(fx0-ux*flame*0.6), float32(fy0-uy*flame*0.6), 2, colGold, true)
}

// drawHUD draws the top bar (file, position) and bottom bar (hints,
// status message).
func (r *Renderer) drawHUD(screen *ebiten.Image, v actions.View, hud HUD) {
	w, h := float32(r.layout.ScreenW), float32(r.layout.ScreenH)
	// Bars.
	vector.DrawFilledRect(screen, 0, 0, w, BarH, colBar, false)
	// Tab strip below the top bar.
	stripY := float32(BarH)
	vector.DrawFilledRect(screen, 0, stripY, w, TabBarHeight, colStrip, false)
	vector.DrawFilledRect(screen, 0, stripY+TabBarHeight-1, w, 1, colBorder, false)
	rects := TabBarRects(tabTitles(hud.Tabs), r.layout.CellW)
	for i, rc := range rects {
		if i >= len(hud.Tabs) {
			break
		}
		rc.X -= hud.TabScroll
		// Cull tabs scrolled entirely out of the strip.
		if rc.X+rc.W <= 0 || rc.X >= r.layout.ScreenW {
			continue
		}
		if hud.Tabs[i].Active {
			vector.DrawFilledRect(screen, float32(rc.X), float32(rc.Y),
				float32(rc.W), float32(rc.H), colMenuSel, false)
			vector.DrawFilledRect(screen, float32(rc.X), float32(rc.Y),
				float32(rc.W), 2, colTitle, false)
		}
		col := colDim
		if hud.Tabs[i].Active {
			col = colText
		}
		r.drawText(screen, hud.Tabs[i].Title,
			float64(rc.X+10), float64(rc.Y+2), col)
	}
	vector.DrawFilledRect(screen, 0, h-BarH, w, BarH, colBar, false)
	vector.DrawFilledRect(screen, 0, BarH-1, w, 1, colBorder, false)
	vector.DrawFilledRect(screen, 0, h-BarH, w, 1, colBorder, false)

	// Top-left: File/Help menu buttons. (The filename lives in the tab
	// strip below - not repeated here.)
	r.drawMenuButtons(screen, hud)
	// Top-right: sound toggle, then the caret position.
	sb := SoundBtnRect(r.layout.ScreenW)
	label, col := "Sound: on", colText
	if !hud.SoundOn {
		label, col = "Sound: off", colDim
	}
	if hud.Menu != nil && hud.Menu.HoverBtn == HoverSound {
		vector.DrawFilledRect(screen, float32(sb.X-3), float32(sb.Y),
			float32(sb.W), float32(sb.H), colMenuSel, false)
	}
	r.drawText(screen, label, float64(sb.X), 8, col)

	// Top-right: caret position.
	pos := fmt.Sprintf("Ln %d  Col %d", v.Caret.Line+1, v.Caret.Col+1)
	r.drawTextRight(screen, pos, r.layout.ScreenW-8, 8, colDim)

	// Bottom-left: the chat draft line while chatting (no key hints on
	// screen - the help dialog documents the bindings).
	if hud.ChatDraft != nil {
		r.drawText(screen, "SAY: "+*hud.ChatDraft+"_", 8,
			float64(r.layout.ScreenH-BarH+8), colStatus)
	}

	// Bottom-right: status message.
	if v.Status != "" {
		r.drawTextRight(screen, v.Status, r.layout.ScreenW-8,
			r.layout.ScreenH-BarH+8, colStatus)
	}
}

// drawMenuButtons paints the File/Help buttons in the top bar, with the
// hovered or open button highlighted.
func (r *Renderer) drawMenuButtons(screen *ebiten.Image, hud HUD) {
	type btn struct {
		rect  Rect
		label string
		id    int
	}
	btns := []btn{
		{FileBtn, "File", 0},
		{HelpBtn, "Help", 1},
		{SettingsBtn, "Settings", HoverSettings},
	}
	for _, b := range btns {
		active := false
		if hud.Menu != nil {
			if (b.id == 0 && hud.Menu.Open == MenuFile) ||
				(b.id == 1 && hud.Menu.Open == MenuHelp) ||
				(b.id == HoverSettings && hud.Menu.Open == MenuSettings) {
				active = true
			}
			if hud.Menu.HoverBtn == b.id {
				active = true
			}
		}
		if active {
			vector.DrawFilledRect(screen, float32(b.rect.X-3), float32(b.rect.Y),
				float32(b.rect.W), float32(b.rect.H), colMenuSel, false)
		}
		r.drawText(screen, b.label, float64(b.rect.X), float32y(b.rect.Y+6), colText)
	}
}

// float32y keeps drawText calls readable (its y is a float64).
func float32y(v int) float64 { return float64(v) }

// drawMenu paints an open dropdown panel beneath its button.
func (r *Renderer) drawMenu(screen *ebiten.Image, hud HUD) {
	if hud.Menu == nil || hud.Menu.Open == MenuNone {
		return
	}
	labels, btn := MenuLabels(hud.Menu.Open)
	rects := DropRects(btn, labels)
	w := DropWidth(labels)
	h := len(labels) * dropItemH
	// Panel + border.
	vector.DrawFilledRect(screen, float32(btn.X-2), float32(DropY-2),
		float32(w+4), float32(h+4), colBorder, false)
	vector.DrawFilledRect(screen, float32(btn.X), float32(DropY),
		float32(w), float32(h), colBar, false)
	for i, rc := range rects {
		if hud.Menu.HoverItem == i {
			vector.DrawFilledRect(screen, float32(rc.X), float32(rc.Y),
				float32(rc.W), float32(rc.H), colMenuSel, false)
		}
		r.drawText(screen, labels[i], float64(rc.X+10), float64(rc.Y+3), colText)
	}
}

// drawDialog paints a centred modal dialog (Help/About).
func (r *Renderer) drawDialog(screen *ebiten.Image, d Dialog) {
	x, y, width, height, titleH, lineH, pad := dialogGeom(
		r.layout.ScreenW, r.layout.ScreenH, d)
	// Frame, body, title bar.
	vector.DrawFilledRect(screen, float32(x-3), float32(y-3),
		float32(width+6), float32(height+6), colBorder, false)
	vector.DrawFilledRect(screen, float32(x), float32(y),
		float32(width), float32(height), colBar, false)
	vector.DrawFilledRect(screen, float32(x), float32(y),
		float32(width), float32(titleH), colTitleBar, false)
	r.drawText(screen, d.Title, float64(x+pad), float64(y+5), colBarText)
	for i, l := range d.Lines {
		col := colText
		if strings.HasPrefix(l, "http") || strings.HasPrefix(l, "Support") {
			col = colStatus // highlight the support line and the link
		}
		r.drawText(screen, l, float64(x+pad), float64(y+titleH+i*lineH), col)
	}
}

// drawDocLine renders one buffer line rune by rune so every glyph is
// optically centred in its grid cell. At the non-default font sizes the
// monospace advance is fractional (e.g. 10.8px at 18px), and centering
// keeps columns on the cell lattice instead of drifting.
func (r *Renderer) drawDocLine(screen *ebiten.Image, s string, x, y float64, col color.Color) {
	cellW := float64(r.layout.CellW)
	for _, ch := range s {
		w := text.Advance(string(ch), r.bufFace)
		r.drawText(screen, string(ch), x+(cellW-w)/2, y, col)
		x += cellW
	}
}

// ScrollbarZones reports whether (sx, sy) lies on the overlay scrollbar
// tracks (vertical = right edge, horizontal = bottom edge of the viewport).
// Bars only exist when the document overflows its viewport, so a short
// document never swallows clicks on the edges. The shell uses this to
// grab the bars and to swallow clicks there.
func (l Layout) ScrollbarZones(sx, sy int, d doc.Document) (vZone, hZone bool) {
	docH := d.LineCount() * l.CellH
	docW := 0
	for i := 0; i < d.LineCount(); i++ {
		if w := d.RuneCount(i) * l.CellW; w > docW {
			docW = w
		}
	}
	if docH > l.ViewH {
		vZone = sx >= l.OriginX+l.ViewW-ScrollbarW && sx < l.OriginX+l.ViewW &&
			sy >= l.OriginY && sy < l.OriginY+l.ViewH
	}
	if docW > l.ViewW {
		hZone = sy >= l.OriginY+l.ViewH-ScrollbarW && sy < l.OriginY+l.ViewH &&
			sx >= l.OriginX && sx < l.OriginX+l.ViewW
	}
	return
}

// drawScrollbars overlays translucent track+thumb bars on the right and
// bottom edges of the text viewport when the document overflows. The
// thumb position mirrors the camera; the shell drives it by locking the
// camera and calling SetScrollYFrac while dragging.
func (r *Renderer) drawScrollbars(screen *ebiten.Image, d doc.Document) {
	l := r.layout
	docH := d.LineCount() * l.CellH
	if docH > l.ViewH { // vertical bar
		x := float32(l.OriginX + l.ViewW - ScrollbarW)
		vector.DrawFilledRect(screen, x, float32(l.OriginY), ScrollbarW, float32(l.ViewH), colScrollTrack, false)
		thumbH := float32(max(ScrollbarW*3, l.ViewH*l.ViewH/docH))
		maxY := float64(docH - l.ViewH)
		frac := clamp(l.ScrollY/maxY, 0, 1)
		ty := float32(l.OriginY) + float32(frac)*(float32(l.ViewH)-thumbH)
		vector.DrawFilledRect(screen, x, ty, ScrollbarW, thumbH, colScrollThumb, false)
	}
	docW := 0
	for i := 0; i < d.LineCount(); i++ {
		if w := d.RuneCount(i) * l.CellW; w > docW {
			docW = w
		}
	}
	if docW > l.ViewW { // horizontal bar
		y := float32(l.OriginY + l.ViewH - ScrollbarW)
		vector.DrawFilledRect(screen, float32(l.OriginX), y, float32(l.ViewW), ScrollbarW, colScrollTrack, false)
		thumbW := float32(max(ScrollbarW*3, l.ViewW*l.ViewW/docW))
		maxX := float64(docW - l.ViewW)
		frac := clamp(l.ScrollX/maxX, 0, 1)
		tx := float32(l.OriginX) + float32(frac)*(float32(l.ViewW)-thumbW)
		vector.DrawFilledRect(screen, tx, y, thumbW, ScrollbarW, colScrollThumb, false)
	}
}

// drawText draws s with its upper-left corner at (x, y). GeoM operations
// are applied in call order: scale the glyph space first, then place it.
// drawText draws s with its upper-left corner at (x, y). There is a single
// GUI face (document + chrome alike), so no glyph-space scaling happens.
func (r *Renderer) drawText(dst *ebiten.Image, s string, x, y float64, col color.Color) {
	op := &text.DrawOptions{}
	op.GeoM.Translate(x, y)
	op.ColorScale.ScaleWithColor(col)
	text.Draw(dst, s, r.bufFace, op)
}

// drawTextRight draws s so that it ends at x (upper edge at y).
func (r *Renderer) drawTextRight(dst *ebiten.Image, s string, x, y int, col color.Color) {
	w := text.Advance(s, r.bufFace)
	r.drawText(dst, s, float64(x)-w, float64(y), col)
}

// drawRuneCentered draws one rune centred on (cx, cy) at the given scale:
// centre the glyph box in local space, scale it, then place it.
func (r *Renderer) drawRuneCentered(dst *ebiten.Image, ch rune, cx, cy, scale float64, col color.Color) {
	s := string(ch)
	w := text.Advance(s, r.bufFace)
	m := r.bufFace.Metrics()
	h := m.HAscent + m.HDescent
	op := &text.DrawOptions{}
	op.GeoM.Translate(-w/2, -h/2)
	op.GeoM.Scale(scale, scale) // relative: 1 = natural document size
	op.GeoM.Translate(cx, cy)
	op.ColorScale.ScaleWithColor(col)
	op.Filter = ebiten.FilterNearest
	text.Draw(dst, s, r.bufFace, op)
}

// runeDrawer adapts drawRuneCentered for the particle renderer, applying
// the world-to-screen offset.
func (r *Renderer) runeDrawer(dst *ebiten.Image, offX, offY float64) func(rune, float64, float64, color.Color) {
	return func(ch rune, x, y float64, col color.Color) {
		r.drawRuneCentered(dst, ch, x+offX, y+offY, 1, col)
	}
}

// drawActor draws one remote gunman: his sprite (mirrored by facing), the
// name tag above his head and, when recent, a cartoon chat bubble over
// that. The bubble fades out over its last two seconds.
func (r *Renderer) drawActor(screen *ebiten.Image, a Actor, offX, offY float64) {
	snap := agent.Snapshot{
		X: a.X, Y: a.Y, Facing: a.Facing, State: a.State, T: a.T,
	}
	id, xOff, yOff := r.sheet().pose(snap)
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(xOff), float64(yOff))
	if a.Facing > 0 {
		// The pixel array is the LEFT-facing base art; facing right is
		// derived by mirroring around the sprite box.
		op.GeoM.Scale(-1, 1)
		op.GeoM.Translate(artW, 0)
	}
	op.GeoM.Scale(float64(Scale), float64(Scale))
	op.GeoM.Translate(a.X+offX, a.Y+offY)
	op.Filter = ebiten.FilterNearest
	screen.DrawImage(r.sheet().image(id), op)

	top := a.Y + offY
	if a.Name != "" {
		w := text.Advance(a.Name, r.bufFace)
		r.drawText(screen, a.Name, a.X+offX+agent.SpriteW/2-w/2, top-float64(r.bufLineH), colActorName)
		top -= float64(r.bufLineH)
	}
	if a.Chat != "" && a.ChatT < BubbleLife {
		r.drawBubble(screen, a.X+offX+agent.SpriteW/2, top-3, Bubble{Text: a.Chat, T: a.ChatT})
	}
}

// BubbleLife is how long a chat balloon stays visible (seconds).
const BubbleLife = 5.0

// drawBubble draws a cartoon chat balloon whose tail points down at (cx,
// anchorY). The last two seconds fade the whole bubble out.
func (r *Renderer) drawBubble(screen *ebiten.Image, cx, anchorY float64, b Bubble) {
	text := b.Text
	if len(text) > 34 {
		text = text[:33] + "~"
	}
	w := float64(len(text)*charW) + 14
	if w < 46 {
		w = 46
	}
	h := 18.0
	x := cx - w/2
	y := anchorY - h - 7
	if x < 4 {
		x = 4
	}
	if x+w > float64(r.layout.ScreenW)-4 {
		x = float64(r.layout.ScreenW) - w - 4
	}

	alpha := uint8(255)
	if fade := BubbleLife - b.T; fade < 2 {
		if fade < 0 {
			fade = 0
		}
		alpha = uint8(255 * fade / 2)
	}
	body := color.RGBA{248, 248, 240, alpha}
	edge := color.RGBA{30, 30, 36, alpha}
	textCol := color.RGBA{20, 20, 26, alpha}

	// Tail: a little stem bump below the balloon.
	vector.DrawFilledCircle(screen, float32(cx), float32(y+h-1), 6, edge, true)
	vector.DrawFilledCircle(screen, float32(cx), float32(y+h-3), 5, body, true)
	// Frame + body.
	vector.DrawFilledRect(screen, float32(x-2), float32(y-2),
		float32(w+4), float32(h+4), edge, false)
	vector.DrawFilledRect(screen, float32(x), float32(y),
		float32(w), float32(h), body, false)
	r.drawText(screen, text, x+7, y+5, textCol)
}

// tabTitles extracts titles for hit-testing geometry.
func tabTitles(tabs []TabInfo) []string {
	out := make([]string, len(tabs))
	for i, t := range tabs {
		out[i] = t.Title
	}
	return out
}

// katanaPivotX is the blade origin during a slash. The sword lives in the
// sheath on his back (the back edge of the sprite, ~5 art px off centre):
// the pivot starts at the sheath (t ~ 0) and slides into his front hands
// as he draws and cuts (t >= ~0.14), so the blade ends pointing at the
// struck glyph at eye height.
func katanaPivotX(a agent.Snapshot, offX float64) float64 {
	dir := float64(a.Facing)
	backX := a.X + agent.SpriteW/2 - dir*5*float64(Scale)
	frontX := a.X + agent.SpriteW/2 + dir*5*float64(Scale)
	k := a.T / 0.14
	if k > 1 {
		k = 1
	}
	k = k * k * (3 - 2*k) // smoothstep
	return backX + (frontX-backX)*k + offX
}
