// Command duke runs the gunman text editor: a notepad with no cursor,
// where a small Duke Nukem-style character lives inside the buffer. He
// walks to wherever you point him, stamps letters in when you type, shoots
// glyphs out when you delete, drags lines when you reorder them and
// celebrates when the file is saved.
//
// This package is the platform shell only: it polls Ebitengine's raw input,
// publishes semantic events on the bus and hands frames to the renderer.
// All editor logic lives in internal/.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"image"
	"image/png"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"shooter/internal/actions"
	"shooter/internal/audio"
	"shooter/internal/doc"
	"shooter/internal/events"
	"shooter/internal/fileio"
	"shooter/internal/netplay"
	"shooter/internal/render"
	"shooter/internal/ui"
)

const (
	frameDT       = 1.0 / 60.0 // fixed timestep: Ebitengine Update runs at 60 TPS
	repeatDelay   = 0.30       // held-key delay before repeating
	repeatRate    = 0.07       // interval between repeats
	mouseFireRate = 0.32       // held right-button rapid-fire interval
	tabWheelStep  = 24         // px per wheel notch when scrolling the tab strip
	dumpFrames    = 90         // frames rendered before a -dump screenshot
)

// errDone quits the game loop cleanly (used by -dump).
var errDone = errors.New("frame dump complete")

// errQuit is returned when File > Close asks the app to exit.
var errQuit = errors.New("quit requested")

// repeatKey tracks one watched key's OS-style repeat state.
type repeatKey struct {
	held       float64
	since      float64
	firedDelay bool
}

// dragState tracks an in-progress scrollbar drag (left button on an
// overlay bar): vertical tells whether the vertical or horizontal bar
// was grabbed.
type dragState struct {
	held     bool
	vertical bool
}

// watchedKeys are the keys that emit KeyPressed events, repeating while
// held. Letters appear here too so Ctrl chords (Ctrl+S/O/Z/Y) repeat; the
// editor ignores plain presses of those.
var watchedKeys = []ebiten.Key{
	ebiten.KeyLeft, ebiten.KeyRight, ebiten.KeyUp, ebiten.KeyDown,
	ebiten.KeyHome, ebiten.KeyEnd, ebiten.KeyPageUp, ebiten.KeyPageDown,
	ebiten.KeyBackspace, ebiten.KeyDelete, ebiten.KeyEnter, ebiten.KeyTab,
	ebiten.KeyF1, ebiten.KeyF2, ebiten.KeyEscape,
	ebiten.KeyS, ebiten.KeyO, ebiten.KeyZ, ebiten.KeyY,
	ebiten.KeyK, ebiten.KeyU, ebiten.KeyA, ebiten.KeyE,
	ebiten.KeyT, ebiten.KeyW,
}

// game wires the slices together and implements ebiten.Game.
type game struct {
	bus    events.Bus
	doc    doc.Document
	engine actions.Engine
	rend   *render.Renderer
	editor *ui.Editor
	path   string

	view      actions.View
	synth     *audio.Synth
	tabs      *ui.Tabs // shared tab strip
	nc        *netCtl  // LAN controller (nil in solo play)
	keys      map[ebiten.Key]*repeatKey
	rightHeld float64   // held right-button accumulator (rapid fire)
	scroller  dragState // in-progress scrollbar drag
	dump      string
	frames    int
	done      bool
	quit      bool
}

// Update polls input, advances the simulation and the camera once frame.
func (g *game) Update() error {
	if g.done {
		return errDone
	}
	if g.quit {
		log.Printf("bye")
		return errQuit
	}
	// The active tab drives the document/path used everywhere else.
	if cur := g.tabs.Cur(); cur != nil {
		g.doc, g.path = cur.Doc, cur.Path
	}
	g.collectInput(frameDT)
	// Cursor position every frame (menu hovers).
	mx, my := ebiten.CursorPosition()
	g.bus.Publish(events.MouseMoved{X: mx, Y: my})
	g.engine.Tick(frameDT)
	g.view = g.engine.View() // drains this frame's FX effects
	g.nc.Update(frameDT, g.doc, g.engine, g.view)
	if g.nc.disconnectedOnce() {
		g.engine.SetStatus("HOST DISCONNECTED")
	}
	for _, snd := range g.view.Sounds {
		g.synth.Play(snd)
	}

	g.rend.Update(frameDT, g.doc, g.view)
	return nil
}

// Draw renders the frame; with -dump it also saves a screenshot and asks
// the loop to quit.
func (g *game) Draw(screen *ebiten.Image) {
	hud := render.HUD{
		SoundOn:   !g.synth.Muted(),
		Menu:      g.editor.MenuState(),
		Dialog:    g.editor.Dialog(),
		ChatDraft: g.editor.ChatDraft(),
		TabScroll: g.tabs.ScrollX,
	}
	if g.nc != nil {
		hud.Actors = g.nc.actors()
		hud.LocalChat = g.nc.localChat
	}
	for i, tab := range g.tabs.Items {
		title := render.TabTitle(tab.Path)
		if tab.Doc.Dirty() {
			title += " *"
		}
		hud.Tabs = append(hud.Tabs, render.TabInfo{
			Title:  title,
			Active: i == g.tabs.Active,
		})
	}
	g.rend.Draw(screen, g.doc, g.view, hud)
	g.frames++
	if g.dump != "" && g.frames >= dumpFrames {
		if err := savePNG(screen, g.dump); err != nil {
			log.Printf("dump failed: %v", err)
		} else {
			log.Printf("dumped screenshot to %s", g.dump)
		}
		g.done = true
	}
}

// Layout returns the fixed internal resolution (the window scales it).
func (g *game) Layout(width, height int) (int, int) {
	return g.rend.ScreenSize()
}

// collectInput turns raw Ebitengine input into bus events: typed runes,
// key presses (with OS-style repeat) and mouse clicks.
func (g *game) collectInput(dt float64) {
	ctrl := ebiten.IsKeyPressed(ebiten.KeyControlLeft) || ebiten.IsKeyPressed(ebiten.KeyControlRight)
	shift := ebiten.IsKeyPressed(ebiten.KeyShiftLeft) || ebiten.IsKeyPressed(ebiten.KeyShiftRight)
	alt := ebiten.IsKeyPressed(ebiten.KeyAltLeft) || ebiten.IsKeyPressed(ebiten.KeyAltRight)

	// Ctrl+M toggles the mute (handled here so plain "m" still types).
	if ctrl && inpututil.IsKeyJustPressed(ebiten.KeyM) {
		if g.synth.ToggleMute() {
			log.Printf("sound muted")
		} else {
			log.Printf("sound on")
		}
	}

	// Text input. Ctrl chords are skipped entirely (Ctrl+S must not type
	// 's'); control codes and tab are handled as keys instead.
	if !ctrl {
		for _, r := range ebiten.AppendInputChars(nil) {
			if r < 32 || r == 127 || r == '\t' {
				continue
			}
			g.bus.Publish(events.RuneTyped{Ch: r})
		}
	}

	// Key presses with OS-style key repeat.
	for _, k := range watchedKeys {
		st := g.keys[k]
		ev := events.KeyPressed{Key: k, Ctrl: ctrl, Shift: shift, Alt: alt}
		switch {
		case inpututil.IsKeyJustPressed(k):
			g.bus.Publish(ev)
			*st = repeatKey{}
		case ebiten.IsKeyPressed(k):
			st.held += dt
			if !st.firedDelay {
				if st.held >= repeatDelay {
					g.bus.Publish(ev)
					st.firedDelay = true
					st.since = 0
				}
			} else if st.since += dt; st.since >= repeatRate {
				g.bus.Publish(ev)
				st.since = 0
			}
		default:
			*st = repeatKey{}
		}
	}

	// Mouse: left walks there, right shoots the clicked glyph. Clicks that
	// land on the overlay scrollbars are swallowed here (the shell owns
	// the camera) unless a dialog/menu/chat is open.
	mx, my := ebiten.CursorPosition()
	vZone, hZone := false, false
	if g.editor.Dialog() == nil && g.editor.MenuState().Open == render.MenuNone &&
		g.editor.ChatDraft() == nil {
		vZone, hZone = g.rend.Layout().ScrollbarZones(mx, my, g.doc)
	}
	for _, btn := range []ebiten.MouseButton{ebiten.MouseButtonLeft, ebiten.MouseButtonRight} {
		if inpututil.IsMouseButtonJustPressed(btn) {
			if vZone || hZone {
				continue // the scrollbar owns this press
			}
			g.bus.Publish(events.MousePressed{X: mx, Y: my, Button: btn})
		}
	}

	// Mouse wheel over the tab strip scrolls it (sublime-style) when the
	// tabs overflow; wheel up reveals earlier tabs.
	if _, wy := ebiten.Wheel(); wy != 0 {
		_, my := ebiten.CursorPosition()
		if my >= render.BarH && my < render.BarH+render.TabBarHeight &&
			g.editor.Dialog() == nil && g.editor.MenuState().Open == render.MenuNone {
			g.tabs.ScrollBy(-int(wy)*tabWheelStep, g.rend.Layout().CellW, g.rend.Layout().ScreenW)
		}
	}

	// Scrollbar dragging: grabbing an overlay bar freezes that axis and
	// follows the cursor; releasing returns the camera to the caret.
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) && (vZone || hZone) {
		if !g.scroller.held {
			g.scroller.held = true
			g.scroller.vertical = vZone
			if vZone {
				g.rend.Layout().LockScrollY()
			} else {
				g.rend.Layout().LockScrollX()
			}
		}
	} else if g.scroller.held {
		l := g.rend.Layout()
		l.UnlockScrollY()
		l.UnlockScrollX()
		g.scroller.held = false
	}
	if g.scroller.held {
		l := g.rend.Layout()
		if g.scroller.vertical {
			l.SetScrollYFrac(float64(my-l.OriginY)/float64(l.ViewH), g.doc)
		} else {
			l.SetScrollXFrac(float64(mx-l.OriginX)/float64(l.ViewW), g.doc)
		}
	}

	// Holding the right button keeps firing at the cursor (rapid fire).
	// The engine's cooldown paces the actual shots once the command
	// queue is drained; this only reissues the click at the pistol's
	// natural cadence so a held button reads as steady fire.
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonRight) {
		if vZone || hZone {
			g.rightHeld = 0
		} else {
			g.rightHeld += dt
			if g.rightHeld >= mouseFireRate {
				g.rightHeld = 0
				g.bus.Publish(events.MousePressed{X: mx, Y: my, Button: ebiten.MouseButtonRight})
			}
		}
	} else {
		g.rightHeld = 0
	}
}

// savePNG writes the current frame as a PNG (debug frame-dump support).
func savePNG(screen *ebiten.Image, path string) error {
	b := screen.Bounds()
	pix := make([]byte, 4*b.Dx()*b.Dy())
	screen.ReadPixels(pix)
	img := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	copy(img.Pix, pix)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// settingsFile stores the last sheet + font size the player picked (cwd).
const settingsFile = "settings.json"

// loadSettings restores the persisted player choices, if any.
func loadSettings(rend *render.Renderer) {
	b, err := os.ReadFile(settingsFile)
	if err != nil {
		return
	}
	var s struct {
		Sheet    string
		FontSize int
	}
	if json.Unmarshal(b, &s) != nil {
		return
	}
	if s.Sheet != "" {
		rend.SetSheet(s.Sheet)
	}
	if s.FontSize != 0 {
		rend.SetFontSize(s.FontSize)
	}
}

// saveSettings persists the chosen sheet and font size for the next launch.
func saveSettings(sheet string, fontSize int) {
	b, err := json.Marshal(struct {
		Sheet    string
		FontSize int
	}{sheet, fontSize})
	if err != nil {
		return
	}
	_ = os.WriteFile(settingsFile, b, 0o644)
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	dump := flag.String("dump", "",
		"render the scene, save a screenshot PNG to this path, then exit (debug)")
	serveAddr := flag.String("serve", "", "host a LAN session (e.g. :3310)")
	joinAddr := flag.String("join", "", "join a LAN session (host:port)")
	name := flag.String("name", "", "participant name shown above your gunman")
	botFlag := flag.Bool("bot", false, "join as the scripted test bot (needs -join)")
	synthFlag := flag.Bool("synth", false,
		"use the built-in sound synthesizer instead of WAV samples (optional engine)")
	flag.Parse()

	pname := *name
	if pname == "" {
		if *botFlag {
			pname = "DUKE-BOT"
		} else {
			pname = "DUKE"
		}
	}
	if *botFlag && *joinAddr == "" {
		log.Fatal("-bot requires -join")
	}

	path := "untitled.txt"
	if flag.NArg() > 0 {
		path = flag.Arg(0)
	}

	rend, err := render.New()
	if err != nil {
		log.Fatalf("renderer: %v", err)
	}
	loadSettings(rend) // Settings > Sprite Sheet / Font Size persist across runs

	var d doc.Document = doc.New()

	// Tab strip (tab 0 = the CLI file; Ctrl+T opens local scratch tabs).
	tabs := &ui.Tabs{Items: []*ui.Tab{{Doc: d, Path: path}}}

	// LAN session (optional): wrap the buffer so every mutation ships to
	// the host before the rest of the program is wired up.
	var nc *netCtl
	joining := *joinAddr != ""
	if *serveAddr != "" || joining {
		var (
			sess netplay.Session
			host *netplay.Host
			err  error
		)
		if *serveAddr != "" {
			host, err = netplay.Listen(*serveAddr)
			if err != nil {
				log.Fatalf("serve: %v", err)
			}
			sess = host
			log.Printf("hosting LAN session on %s as %q", host.Addr(), pname)
		} else {
			sess, err = netplay.Dial(*joinAddr, pname, newParticipantID())
			if err != nil {
				log.Fatalf("join: %v", err)
			}
			log.Printf("joining %s as %q", *joinAddr, pname)
		}
		var ncRef *netCtl
		obs := netplay.NewObservedDoc(d, func(op netplay.DocOp) {
			if ncRef != nil {
				ncRef.emit(op)
			}
		})
		nc = newNetCtl(sess, host, obs, rend.Layout().Grid, pname, *botFlag)
		ncRef = nc
		d = obs
	}

	engine := actions.New(d, rend.Layout().Grid)
	bus := events.NewBus()
	store := fileio.OSStore{}
	var netIF ui.Messenger
	if nc != nil {
		netIF = nc
	}
	// Sound engine: WAV samples by default (silent until files exist in
	// the sounds/ directory); -synth selects the built-in synthesizer.
	var sound *audio.Synth
	if *synthFlag {
		sound = audio.NewSynth()
	} else {
		sound = audio.NewSamples(audio.SoundsDir)
	}
	log.Printf("sound: %s", sound.Mode())
	editor := ui.New(ui.Services{
		Bus:          bus,
		Doc:          d,
		Engine:       engine,
		Store:        store,
		Layout:       rend.Layout(),
		Path:         path,
		Net:          netIF,
		Multiplayer:  nc != nil,
		OpenURL:      openBrowser,
		PickSavePath: pickSavePath,
		PickOpenPath: pickOpenPath,
		ToggleMute: func() bool {
			return sound.ToggleMute()
		},
		Sheets: func() ([]string, string) {
			return rend.SheetNames(), rend.ActiveSheet()
		},
		SetSheet: func(name string) bool {
			if !rend.SetSheet(name) {
				return false
			}
			saveSettings(name, rend.FontSize())
			return true
		},
		FontSizes: func() ([]int, int) {
			return rend.FontSizeList()
		},
		SetFontSize: func(size int) bool {
			if !rend.SetFontSize(size) {
				return false
			}
			engine.SetGrid(rend.Layout().Grid)
			saveSettings(rend.ActiveSheet(), size)
			// The font size rebuilds the canvas (the 80x20 grid keeps only
			// this shape at the OLD window size, which would letterbox):
			// re-fit the window to the new canvas edge-to-edge.
			w, h := rend.ScreenSize()
			if m := ebiten.Monitor(); m != nil {
				mw, mh := m.Size()
				w, h = fitWindowToMonitor(w, h, mw, mh)
			}
			ebiten.SetWindowSize(w, h)
			return true
		},
		Tabs: tabs,
	})
	if !joining {
		// Solo/host: load our file. Clients take the host's buffer.
		editor.LoadFile()
	}

	g := &game{
		bus:    bus,
		doc:    d,
		engine: engine,
		rend:   rend,
		editor: editor,
		synth:  sound,
		tabs:   tabs,
		nc:     nc,
		path:   path,
		keys:   map[ebiten.Key]*repeatKey{},
		dump:   *dump,
	}
	for _, k := range watchedKeys {
		g.keys[k] = &repeatKey{}
	}
	// File > Close asks the shell to exit.
	bus.Subscribe(func(e events.Event) {
		if _, ok := e.(events.QuitRequested); ok {
			g.quit = true
		}
	})

	w, h := rend.ScreenSize()
	// Fit the window to the monitor: a window clamped by the OS gets
	// black letterbox bars (the fixed canvas cannot stretch), so scale
	// the initial size down, preserving the aspect. Window resizing is
	// disabled afterwards - the pixel grid is fixed and letterboxing
	// only ever reads as broken.
	if m := ebiten.Monitor(); m != nil {
		mw, mh := m.Size()
		w, h = fitWindowToMonitor(w, h, mw, mh)
	}
	ebiten.SetWindowSize(w, h)
	ebiten.SetWindowTitle("DUKE - gunman text editor [" + filepath.Base(path) + "]")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeDisabled)
	log.Printf("start: file=%s window=%dx%d", path, w, h)

	runErr := ebiten.RunGame(g)
	nc.Close() // nil-safe: solo play does nothing
	if runErr != nil && !errors.Is(runErr, errDone) && !errors.Is(runErr, errQuit) {
		log.Fatal(runErr)
	}
}

// fitWindowToMonitor scales the canvas size (w, h) down, preserving the
// aspect, so the window fits entirely on the screen with room for the
// taskbar. Returns the input unchanged when it already fits.
func fitWindowToMonitor(w, h, mw, mh int) (int, int) {
	if mw <= 0 || mh <= 0 {
		return w, h
	}
	const taskbarMargin = 96 // breathing room for the taskbar/dock
	availH := mh - taskbarMargin
	if availH <= 0 {
		return w, h
	}
	scale := math.Min(float64(mw)/float64(w), float64(availH)/float64(h))
	if scale >= 1 {
		return w, h
	}
	fw := int(math.Round(float64(w) * scale))
	fh := int(math.Round(float64(h) * scale))
	if fw < 640 || fh < 480 {
		return w, h // degenerate screen: keep the nominal size
	}
	return fw, fh
}

// openBrowser launches a URL in the platform browser. Uses only stock OS
// tools (rundll32 on Windows, open on macOS, xdg-open elsewhere).
func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
