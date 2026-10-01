# SPEC — DUKE: the gunman text editor

Software Requirements Specification and architecture reference.
Module: `shooter` (Go 1.26, Ebitengine v2.10.4). Version 1.0.

---

## 1. Product vision

A notepad-like text editor **with no cursor**. Instead of a blinking caret,
a Duke Nukem-style gunman *lives inside the text buffer*:

- typing text makes him **throw letters** that fly and stamp into the buffer;
- attacking a glyph picks the weapon **by proximity** (the Duke rules):
  within the katana''s reach (Chebyshev <= 4 cells) he swings a **giant
  katana** (a vector blade sweeping a pale arc, far bigger than the
  sprite); anything farther gets the **pistol** (flight time scales with
  distance). Backspace/Delete attack the neighbouring glyphs, right-click
  attacks any glyph on screen — he always fires **from his current
  position**, the caret only moves as a consequence of the deletion;
- the arsenal extends with emacs/shell-style chords: **Ctrl+Backspace /
  Ctrl+Delete fire the shotgun** (word kills), **Ctrl+K fires the rocket
  launcher** (emacs kill-line), **Ctrl+U rockets backwards** (shell
  kill-to-start); holding a chord auto-fires;
- every weapon has a **synthesised sound** (whoosh/crack/boom/launch/
  stamp/chime) — six WAV samples committed under sounds/, extracted from Sprite Fusion's destroy.spritefusion.com chip atlas (`scripts/fetch-sfx.ps1`; synth alternative via `cmd/gensounds`); Ctrl+M mutes;
- arrow keys / mouse clicks make him **walk** — he *is* the caret;
- Ctrl+Up/Down makes him **grab the current line with his hands and
  drag** it past its neighbour (both rows slide);
- Ctrl+S makes him **celebrate** while the file is written.

He is drawn as Duke Nukem: platinum blond flat-top, black wraparound
sunglasses, red tank top, gold Nuke belt buckle, blue jeans and black
steel-tipped boots.

The editor is otherwise a serious little notepad: plain-text files,
undo/redo, a status bar, a help overlay.

## 2. Goals / non-goals

**Goals (v1)**
- FR-1: open a `.txt` file (CLI argument, default `untitled.txt`), edit it,
  save it with Ctrl+S; reload from disk with Ctrl+O.
- FR-2: every editing action is performed *visibly by the gunman* — no
  mutation happens without a corresponding animation.
- FR-3: feel responsive: input is never lost, chained arrow presses and
  clicks retarget the agent immediately.
- FR-4: fully self-contained: no external assets (fonts/sprites/audio are
  embedded or procedural), nothing outside the working directory.
- FR-5: every interface unit-tested without a GPU (174 tests).

**Non-goals (v1)**
- Syntax highlighting, word wrap, line numbers, search, multiple buffers.
- Wide (CJK) glyph support (JetBrains Mono shows placeholder boxes for CJK; column
  alignment is preserved), IME candidate windows, clipboard/paste.
- Audio. Mobile/web packaging.

## 3. Prior-art review (searched at session start)

| Project | Relevance |
|---|---|
| `hajimehoshi/ebiten` (Ebitengine v2) | the engine used here |
| JetBrains Mono (internal/fonts) | embedded OFL coding font: UTF-8 text, document face sized so one glyph advance == one cell |
| `tinne26/ptxt`, `tinne26/etxt` | pixel-font renderers for Ebitengine (alternative font stacks) |
| `ebitengine/exp/textinput` | experimental text-input handling (not needed; `AppendInputChars` suffices) |

No existing "character physically edits the text" editor was found — the
interaction model below is original.

## 4. Architecture

Vertical slices under `internal/`, one interface boundary each, wired by a
service locator (`ui.Services`) in the shell (`cmd/duke`).

```
             ┌──────────────────────── cmd/duke (platform shell) ────────────────┐
             │  polls ebiten input, key-repeat, mouse; implements ebiten.Game       │
             └───────────────┬──────────────────────────────────────┬───────────────┘
                             │ events.RuneTyped / KeyPressed /      │ per frame:
                             │ MousePressed                         │ engine.Tick ->
                             ▼                                      │ engine.View()
                    ┌─────────────────┐                             │ renderer.Update
                    │  events.Bus     │ (sync pub/sub)              ▼
                    │  subscribe/     │                    ┌──────────────────┐
                    │  publish        │                    │ actions.View      │
                    └───────┬─────────┘                    │ snapshot: agent,  │
                            ▼                              │ letters, bullets, │
                    ┌─────────────────┐   commands         │ swaps, status, FX │
                    │  ui.Editor      │──────────────►┌────┴─────────────────┐│
                    │  key mapping,   │               │ actions.Engine       ││
                    │  save/reload,   │◄──────────────│ FIFO queue, worlds,  ││
                    │  help overlay   │  Status/View  │ caret, cooldowns     ││
                    └───────┬─────────┘               └───┬──────┬───────┬───┘│
                            │ Store.Read/Write            │      │       │    │
                            ▼                      ┌───────▼──┐ ┌─▼─────┐ ┌▼─────┐
                    ┌─────────────────┐             │ doc.     │ │agent. │ │fx.   ││
                    │ fileio.Store    │             │ Buffer   │ │Agent  │ │Effect││
                    │ (OS / fake)     │             │ undo/redo│ │ FSM   │ │queue ││
                    └─────────────────┘             └──────────┘ └───────┘ └──────┘│
                            │                                                    │
                            │                      grid.Grid (cell <-> world px) │
                            ▼                      shared by engine + renderer    │
                    ┌────────────────────────────────────────────────────────────┘
                    │ render.Renderer: Layout(camera), sprite sheet, particles,
                    │ text draw, HUD — pure view, never mutates state
                    └──────────────► ebiten screen (960x576, fixed)
```

### Package responsibilities

| Package | Owns | Depends on |
|---|---|---|
| `internal/doc` | line buffer, edit ops, undo/redo, dirty flag | – |
| `internal/events` | event types + synchronous bus | ebiten (key enums only) |
| `internal/grid` | cell ↔ world-pixel mapping | doc |
| `internal/agent` | gunman position/facing/animation FSM | – |
| `internal/fx` | pure effect descriptions (Shatter/Stamp/Muzzle/…) | – |
| `internal/actions` | command queue, projectiles, caret, statuses, FX | doc, agent, grid, fx |
| `internal/render` | layout/camera, sprites, particles, text, HUD | doc, grid, agent, actions, fx |
| `internal/fileio` | `Store` interface + OS implementation | – |
| `internal/ui` | input→command mapping, save/reload, help | doc, actions, events, fileio, render (layout type) |
| `cmd/duke` | Ebitengine game loop, raw input polling, wiring | everything |

Dependency direction never cycles: `actions` knows nothing of `render`;
the renderer consumes `actions.View` snapshots.

## 5. Coordinate spaces

Three spaces, all pixel-based (no world-to-screen scaling; the window
scales the fixed 960x576 surface):

1. **Cell** — `doc.Pos{Line, Col}`, zero-based, *rune* columns.
2. **World pixels** — the document grid: `x = col*CellW`, `y = line*CellH`.
   Where the gunman walks and bullets fly.
3. **Screen pixels** — `world - camera + viewport origin`.
   `Layout.ScrollX/ScrollY` is the camera; `OriginY = 32` (top HUD bar).

Grid derivation (measured from `bitmapfont.Face` at startup):

```
advance("M") = 6px, HAscent = 12, HDescent = 4      (bitmapfont, 1x)
Scale        = 2                                    (font pixel scale)
CellW        = 6 * 2   = 12px     cellH = (12+4) * 2 = 32px
viewport     = 80 cols x 16 rows  = 960 x 512
window       = 960 x (512 + 2*32 HUD bars) = 960 x 576
sprite art   = 12 x 16px, drawn at Scale 2 -> 24 x 32px on screen:
               exactly one row tall (feet on the row's bottom edge, the
               head never clips under the HUD), two cells wide, and its
               pixels are the same size as the font's pixels
```

**Why it matters:** the gunman is *big* (double the original v1 sprite)
while still never crossing a row boundary. He covers his own cell plus the
cell to his right; the glyph he is about to shoot (to his left) always
stays visible, and during forward typing the right-hand cell is the still
empty future, so the occlusion cost is confined to mid-line navigation
(accepted limitation, see §15).

Agent anchor: sprite top-left `y = (line+1)*CellH - SpriteH` (feet rest on
the row's bottom edge; with SpriteH == CellH this is `line*CellH`);
`x = col*CellW`. The gun tip (muzzle) is at mid sprite height.

## 6. Core invariants (the contract)

1. **Caret = agent cell.** `actions.engine.caret` is the insertion point;
   whenever the agent is idle and off the caret he walks back to it
   (`followCaret`).
2. **Mutations are serialized.** A new command only *starts* when: no
   letter in flight, no bullet in flight, no line swap running, no locked
   agent animation (aim/fire/recoil/win) and no fire cooldown. Walks are
   not blocking.
3. **Projectiles capture their targets at spawn.** A letter records the
   target cell + caret when thrown; a shot records the caret base when the
   aim begins. A caret jump mid-flight can therefore never retarget or
   corrupt them; the caret is only restored by the shot if it has not
   moved since the aim.
4. **Walks are never queued.** `WalkTo` moves the caret and retargets the
   agent immediately (they mutate no document state), which keeps chained
   arrow keys and clicks responsive and naturally FIFO for subsequent
   typed runes.
5. **The queue is FIFO** and bounded (`maxQueue=256`; shots additionally
   capped at `maxShootQ=6` so held-backspace degrades into steady
   automatic fire instead of a backlog).
6. **FX are drained once per frame**: `engine.View()` hands the pending
   effects to the renderer and clears them.

## 7. State machines

### Gunman FSM (`internal/agent`)

```
              WalkTo                  Aim(facing)        Slash(facing)
  ┌──────┐ ──────────► ┌──────┐ ──────► ┌─────┐ AimDur  ┌──────┐ FireDur ┌─────────┐
  │ Idle │             │ Walk │         │ Aim │ ──────► │ Fire │ ──────► │ Recoil  │
  └──────┘ ◄────────── └──────┘         └─────┘  EvFired└──────┘         └────┬────┘
     ▲    arrival/retarget   │                                DragTo ─┐       │
     │                       │  WalkTo cancels aim/slash;             ▼       │
     │ WinDur                │  EvArrived on arrival           ┌──────┐ idle  │
 ┌──────┐                    │                                 │ Drag │ after │
 │ Win  │ ◄── Celebrate      │                                 └──────┘Recoil │
 └──────┘                    └────► ┌──────┐  SlashHit  ┌───────┐  Dur      │
                                    │Slash │ ─────────► │EvSlash│ ──────────┘
                                    └──────┘            │applies│  SlashDur ->
                                       (melee hit,      └───────┘  idle
                                        no projectile)
```

Locked states (block new commands): Aim, Fire, Recoil, Slash, Win.
Events returned by `Tick`: `EvArrived`, `EvFired` (spawn the projectile
at `Muzzle()`), `EvSlash` (apply the katana hit directly, no projectile).

**Orientation rule (v2, after play-testing):** facing reflects the
direction of travel and of every attack. Walking left turns him left
and walking right turns him right (walk() derives facing from dx);
backspace strikes face the glyph behind/above him; right-click
attacks face toward the clicked cell horizontally; heavy weapons face
their frozen aim (Ctrl+U left, Ctrl+K right); vertical or own-cell
strikes default to facing left. SetPos (teleport/load) resets him to
the default right-facing stance.

**Walk cycle:** four beats - contact A (left foot planted, right foot
lifted, no sole under it), passing (legs together, body rides one pixel
up), contact B (mirrored: right planted, left lifted), passing.
pose() maps frames 1/3 to a -1px bob; the two contact frames differ in
the sole row, pinned by a test.

### Weapon choice (`internal/actions`)

Every attack freezes a `pendingShot{base, target, aim, from, to,
weapon, ...}` when it starts. Regular shots pick the weapon by Chebyshev
distance from the caret (base) to the target: `<= katanaReach` (4 cells)
= **katana** (`Slash`, drawn from the back sheath, connects on `EvSlash`, no projectile); farther =
**machine gun** (`AimWith(Pistol)` — the enum/asset keep the name `pistol`, the art is an SMG — spawns on `EvFired`). `ShootWord` always
freezes a word range and uses the **shotgun** (pellet spread);
`KillLine` freezes a line range and uses the **rocket launcher** (slow
round, explosion FX). Line drags use the katana stance pose (hands on
the hilt). He always attacks **from his current position**: the caret
only moves as a consequence of the deletion (`shiftCaret`).

### Editor modes (`internal/ui`)

```
  ┌────────┐  F1   ┌──────┐  F1/Esc  ┌────────┐
  │ Normal │ ────► │ Help │ ───────► │ Normal │   (Help swallows all input)
  └────────┘ ◄──── └──────┘          └────────┘
```

### Shot lifecycle (engine-internal)

```
queue: Shoot / ShootAt ─► startShoot: freeze pendingShot{base,aim,...}
       ├─ reach <= katanaReach (4) ─► ag.Slash ─► EvSlash ─► applyHit
       └─ farther ─► ag.AimWith(Pistol) ─► EvFired ─► spawnBullet
queue: ShootWord ─► startSpecial: freeze word range, Shotgun ─► spread
queue: KillLine ─► startSpecial: freeze line range, Rocket ─► explosion
       applyHit ─► doc.Delete(range) ─► shiftCaret ─► Shatter FX
                ─► hitCooldown (+ Explosion/boom for rockets)
```

## 8. Data model & interfaces

```go
// internal/doc
type Pos struct{ Line, Col int }              // rune columns, not bytes
type Hint struct{ Pos Pos; KeepCol bool }      // post undo/redo caret
type Document interface {
    LineCount() int; Line(i int) string; RuneCount(line int) int
    Clamp(p Pos) Pos; Advance(p Pos, s string) Pos
    Insert(p Pos, text string) Pos             // returns end position
    Delete(from, to Pos) string                // returns deleted text
    MoveLine(line, delta int) bool             // single-step adjacent swap
    Text() string; Load(text string)
    Dirty() bool; MarkSaved()
    CanUndo() bool; CanRedo() bool
    Undo() (Hint, bool); Redo() (Hint, bool)
}
```
Storage: `[][]rune` (columns are rune indices by construction). Undo stacks
store the *original* operations; `Undo`/`Redo` invert/replay them in linear
order (insert ⇄ delete, move ⇄ opposite move). `Load` clears both stacks.
CRLF/CR are normalised to LF on `Load`.

```go
// internal/actions
type Engine interface {
    Tick(dt float64)
    TypeRune(r rune); Shoot(back bool); WalkTo(p doc.Pos)
    MoveLine(dir int); Undo(); Redo(); Celebrate(); Cancel()
    Caret() doc.Pos
    View() View        // drains FX; valid until next Tick
    Status() string; SetStatus(msg string)
}
type View struct {                 // per-frame render snapshot
    Agent   agent.Snapshot
    Caret   doc.Pos
    Letters []Letter               // thrown runes in flight (world px)
    Bullets []Bullet               // shots in flight (world px)
    Swaps   []Swap                 // line-drag animations
    Status  string
    FX      []fx.Effect            // drained this frame
}
```

```go
// internal/events
type Bus interface {
    Subscribe(h Handler) (cancel func())
    Publish(e Event)               // sync; snapshot iteration
}
// events: RuneTyped{Ch} | KeyPressed{Key,Ctrl,Shift,Alt} | MousePressed{X,Y,Button}
```

```go
// internal/fileio
type Store interface {
    Read(path string) (string, error)           // missing -> os.ErrNotExist
    Write(path string, content string) error
}
```

```go
// internal/ui  (service locator)
type Services struct {
    Bus events.Bus; Doc doc.Document; Engine actions.Engine
    Store fileio.Store; Layout *render.Layout; Path string
}
```

```go
// internal/render
type Layout struct {
    grid.Grid                       // CellW, CellH
    ScreenW, ScreenH, OriginX, OriginY, ViewW, ViewH int
    ScrollX, ScrollY float64        // camera in world px
}
func (l Layout) ScreenToCell(sx, sy int, d doc.Document) (doc.Pos, bool)
func (l *Layout) follow(cx, cy float64, d doc.Document, dt float64) // easing camera
```

## 9. Interaction specification

| Input | Engine command | Gunman behaviour |
|---|---|---|
| printable rune | `TypeRune` | throws a letter (arcing, ~0.11 s) that stamps into the caret cell; he steps right after each landing |
| Enter | `TypeRune('\n')` | splits the line (letter itself is invisible; stamp FX marks the landing) |
| Tab | 4× `TypeRune(' ')` | four quick letters (keeps rune-column maths simple) |
| Backspace (tap/hold) | `Shoot(true)` | **giant katana swing** on the glyph before the caret when within reach (Chebyshev <= 4 cells — covers the line above), the **machine gun** beyond; hold = automatic flurry |
| Delete | `Shoot(false)` | same proximity rule on the glyph at his feet / next line's start |
| right-click a glyph | `ShootAt(cell)` | attacks that glyph **from where he stands** (katana if within 4 cells, machine gun beyond); the caret does not move |
| Ctrl+Backspace | `ShootWord(true)` | **shotgun**: kills the word before the caret (blanks then word, in-line) |
| Ctrl+Delete | `ShootWord(false)` | shotgun: kills the word ahead + its trailing blanks |
| Ctrl+K | `KillLine(false)` | **rocket launcher**: emacs kill-line — to end of line, or the newline itself at EOL (joins) |
| Ctrl+U | `KillLine(true)` | rocket launcher: shell-style kill from line start to the caret |
| Ctrl+M | mute toggle | shell-side; hidden from the UI (no hint-bar or help mention) by product decision |
| Left/Right | `WalkTo` | one rune; at line edges crosses to the previous/next line end |
| Up/Down | `WalkTo` | same column, clamped to the target line's length |
| Home/End | `WalkTo` | line start / line end |
| PageUp/PageDown | `WalkTo` | ±16 lines |
| left-click in viewport | `WalkTo` | walks there (immediate caret move) |
| click on HUD bar | – | ignored |
| Ctrl+Up / Ctrl+Down | `MoveLine(-1/+1)` | swaps his line with the neighbour; both rows animate, he **drags it with his hands** (bare-fist pose, timed eased slide) |
| Ctrl+Z / Ctrl+Shift+Z / Ctrl+Y | `Undo`/`Redo` | puff FX at the affected spot, walks to the hint position |
| Ctrl+S | save via `Store` | status `SAVED name`, victory pose + confetti; failures show `SAVE FAILED: …` and keep the dirty flag |
| Ctrl+O | reload via `Store` | replaces the buffer, resets the gunman to (0,0); missing file → fresh buffer (`NEW FILE name` only on explicit reload) |
| F1 / Esc | toggle the Help dialog | modal dialog; all other input swallowed while open (any click also closes it) |
| File > New / Ctrl+N | `Doc.Load("")` + `Cancel` | clears the buffer, resets the gunman, status `NEW BUFFER` |
| File > Save / Ctrl+S | saves the ACTIVE TAB to a file | named buffers write in place; untitled buffers open the native save dialog (Windows GetSaveFileNameW / macOS osascript / zenity-kdialog on Linux, working-dir fallback when no dialog exists); the picked path becomes the tab path; cancel keeps dirty and skips the celebration |
| File > Close | `QuitRequested` on the bus | dirty buffer first warns (`UNSAVED CHANGES - CHOOSE CLOSE AGAIN TO DISCARD`), second click quits via `errQuit` |
| Settings > Sprite Sheet | sheet dialog | switches character live; shell persists to settings.json |\n\n| Help > Help Topics | Help dialog | katana/shotgun/rocket/controls reference |
| Help > About | About dialog | shows `Support Xyzzy if you want this to be maintained!` + `https://xyzzy.gumroad.com/l/ykqqqy` + license/copyright lines |
| mouse over menu bar | `MouseMoved` events | hover highlight; open dropdown dismisses on outside click or Esc |
| empty-buffer backspace / buffer-edge line move / empty history | rejected | status message + 0.2 s cooldown (no input flood) |

Key repeat: the shell emulates OS repeat (first repeat after 0.30 s, then
every 0.07 s) and publishes `KeyPressed` — so holding arrows walks in
steps, holding backspace auto-fires (backed by the engine's shot cap).

## 9a. Tabs

`ui.Tabs` (a shared pointer in `ui.Services`, same pattern as
`render.Layout`) owns `[]{Doc, Path, Caret}` plus the active index. The
editor mutates it: `SwitchTab`/`NewTab`/`CloseTab` save the outgoing
caret, then `applyActive` re-points `Services.Doc`/`Path` and calls
`Engine.SwapDocument(doc, caret)` — which drops queued work, clamps the
caret into the new buffer and parks the agent there. Each tab''s document
carries its own undo stack, so history is per-tab for free. The shell
re-syncs its `game.doc`/`game.path` from `Tabs.Cur()` every frame and
renders the strip (geometry + hit-testing in `render/menu.go`:
`TabBarRects`, `TabHit`, `TabTitle`; strip height `TabBarHeight`, which
shifts the viewport origin to 52px). New tabs are local scratch buffers
in LAN mode (only tab 1 is wrapped in the session''s `ObservedDoc`).

## 10. Timing constants (`internal/actions`, seconds)

| Constant | Value | Meaning |
|---|---|---|
| `letterFlight` | 0.11 | thrown letter flight |
| `bulletBase` / `bulletSpeed` | 0.07 s / 2200 px/s | machine-gun round flight (distance-scaled) |
| `shotgunBase` / `shotgunSpeed` | 0.09 s / 1400 px/s | pellet flight (slower, fanned) |
| `rocketBase` / `rocketSpeed` | 0.30 s / 900 px/s | slow, dramatic rocket |
| `hitCooldown` | 0.03 | pause after a mutation applies |
| `failCooldown` | 0.20 | pause after a rejected command |
| `katanaReach` | 4 | cells the giant blade covers (Chebyshev); machine gun beyond |
| `swapDur` | 0.16 | line-drag animation (agent DragTo uses the same duration, so they stay in lockstep) |
| `statusDur` | 1.4 | default status lifetime |
| `maxQueue` / `maxShootQ` | 256 / 6 | queue caps |
| agent | Aim 0.10, Fire 0.05, Recoil 0.09, **SlashHit 0.16, SlashDur 0.30**, Win 0.7, WalkSpeed 160 px/s | |
| shell | repeatDelay 0.30, repeatRate 0.07, dt = 1/60 | fixed timestep |

## 11. Rendering pipeline (per frame)

```
engine.Tick(dt) -> engine.View()            (drains FX once)
renderer.Update(dt, doc, view)               camera follow + particle sim
renderer.Draw(screen, doc, view, hud):
  1. background fill
  2. caret-row highlight (vector rect)
  3. visible buffer lines, each offset by its line-swap animation
  4. gunman sprite (mirror for facing; pose offsets in sprite-local space)
  5. thrown letters (arc + scale pop, rune glyph)
  6. bullets (tracer line + head dot)
  7. particles (shards, glyph fragments, sparks, confetti)
  8. HUD bars: file+dirty | Ln/Col ; hints | status   (covers bleed-over)
  9. help overlay (centred panel)
```

Sprites are ASCII art (`[]string`, 12x24) parsed against a palette at
startup — a validation test rejects wrong dimensions/unknown chars.
GPU images are created lazily on first draw.

Camera: keeps the agent inside the middle 40% of the viewport, eased
(`k = 1 - 0.0001^dt`), clamped so it never scrolls past the document
bounds (computed from the widest line / line count).

**Ebitengine gotcha (documented because it bit us):** `GeoM` operations are
applied *in call order*. Scale must be called **before** Translate to scale
glyph space rather than the translation, and entity drawing must add the
world→screen offset explicitly.

## 12. File handling

- Startup: `Store.Read(path)`; missing file → empty buffer (silent);
  other errors → empty buffer + status.
- Save: write full text, `MarkSaved()`, status + celebration. Dirty flag
  shown as `name *` in the top bar.
- Reload (Ctrl+O): replace buffer, `engine.Cancel()` (drops queue +
  projectiles, caret (0,0), teleports the gunman).
- Encoding: raw UTF-8, LF-normalised on load, written back verbatim.

## 13. Logging (stdlib `log`)

Session start (file, window), save success/failure, load success/failure,
status messages from the engine (rejected commands), Cancel events, frame
dumps. Keystrokes are deliberately *not* logged (spam).

## 14. Testing strategy

- **No GPU in tests.** Every boundary is an interface; UI tests use a mock
  `actions.Engine` + fake `Store` + real bus/doc/layout; engine tests use
  the real document with a fixed grid.
- Deterministic ticking: tests drive `Tick(1/60)` directly.
- Pure logic covered: buffer edits/undo, FSM transitions, projectile
  lifecycles, command FIFO order, queue caps, camera clamping, swap
  offsets, sprite art validation, key mapping table, file round-trips.
- **Visual verification:** `duke -dump out.png file.txt` renders 90
  frames, saves the frame as PNG and exits. Used to find (and prove the
  fix of) the two rendering bugs in §11; screenshots live in
  `docs/screenshot.png`.
- 94 tests across 8 packages; `task check` runs gofmt -l, vet and tests.

## 15. Known limitations (v1)

1. The gunman is 24x32 on screen: he covers his own (caret) cell **and the
   cell to his right**. During forward typing the right cell is empty and
   the just-typed rune reappears ~0.1 s after he steps right (the stamp FX
   keeps feedback visible); while navigating mid-line the glyph to his
   right stays hidden until he moves.
2. Tab inserts 4 spaces instead of a tab character (keeps rune-column
   arithmetic honest).
3. Wide (CJK) glyphs occupy 2 cells but advance is measured for halfwidth;
   Latin/ASCII is the target input.
4. Enter's thrown letter has no visible glyph (control char) — the stamp
   flash marks the line split.
5. A click during a shot's 60 ms flight can leave the caret one delete off
   (positions are clamped, never corrupt); similarly an insert landing
   after a mid-flight click keeps the click's caret (the rune lands where
   it was thrown — FIFO semantics).
6. `Undo` marks the buffer dirty even when returning to the saved state.
7. Camera bounds rescan the document each frame (O(lines); fine for
   notepad-sized files).

## 15a. LAN multiplayer (`-serve` / `-join` / `-bot`)

Topology: one peer hosts (`duke -serve :3310`), others join
(`-join host:port [-name X] [-bot]`). Transport is TCP with
newline-delimited JSON messages (`internal/netplay`: hello/welcome with a
full buffer snapshot, ops, presence at 4 Hz, chat, leave). The host is
authoritative: a peer's own engine applies its edit locally (animations
stay responsive) and the `ObservedDoc` wrapper ships the op to the host,
which replays it (`ApplyRemote` suppresses re-emission, no echo loops) and
relays a sequenced copy to everyone else. Conflicts are last-writer-wins
on clamp (v1; no OT/CRDT). Undo/redo are disabled while connected
(interleaved stacks across peers) with an `UNDO IS OFFLINE ONLY` status.

Presence carries each participant's caret cell, agent state and facing, so
remote gunmen render as extra sprites with name tags. Chat (F2 draft line,
Enter sends, Esc cancels) shows as cartoon bubbles above the sender for
`BubbleLife` (5 s, fading over the last 2 s). The test bot (`-bot`, joins
as DUKE-BOT) runs `bot.Think` — a pure, seedable decision function biased
55% toward destruction (shoot/shotgun/rocket) with trash-talk lines
including "got'em". Disconnects surface as `HOST DISCONNECTED` / peer-left
cleanup. All of it is covered by localhost integration tests
(`internal/netplay`) plus bot determinism tests.

## 16. Build & run

```
task build     # gofmt + vet + tests + bin/duke.exe
task run       # build and run
task run -- notes.txt        # open a file
bin/duke.exe -dump a.png notes.txt   # headless-ish frame dump (debug)
task check     # formatting report + vet + tests
task dist      # + cross-compiled release binaries (see below)
```
Requirements: Go 1.22+ (repo builds with 1.26), Task (optional — plain
`go build ./cmd/duke` works), no C compiler needed: all platforms
build as pure Go (`CGO_ENABLED=0`) because Ebitengine loads its native
libraries dynamically at runtime.

Cross-compilation targets (`task dist` / `task dist-linux` /
`task dist-darwin` / `task dist-darwin-arm64`):

| Binary | Platform |
|---|---|
| `bin/duke.exe` | windows/amd64 |
| `bin/duke-linux-amd64` | linux/amd64 |
| `bin/duke-darwin-amd64` | macOS intel |
| `bin/duke-darwin-arm64` | macOS Apple Silicon |

## 16a. License

PolyForm Noncommercial License 1.0.0 (see `LICENSE`). The required
notice line carries the copyright from this repository's git config:
`Copyright 2026 Xyzzy Apps (xyzzyapps@gmail.com)`. Personal,
noncommercial and noncommercial-organisation use is permitted; anything
commercial needs a separate grant from the licensor.

## 17. Future work (candidates, not committed)

- Scripted `-demo` mode for automated animation verification.
- Sound effects; cartridge/reload flourishes; more sprites (pain, taunt).
- Selection by aiming (mark mode: he shoots a range), clipboard.
- Drag *any* line (not just the caret's) via click-drag on the gutter.
- Tab stops, word wrap, search.
- Undo visual reverse-play (bullets fly backwards, letters unstamp).
