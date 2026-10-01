# TODO — Shooter: the gunman text editor

A notepad-like editor with no cursor. A Duke Nukem-style gunman walks the
buffer, throws letters in when you type, shoots glyphs out when you delete,
and drags lines when you reorder them.

## Phase 1 — Scaffold
- [x] `go mod init shooter`, dependency fetch (ebiten/v2 v2.10.4, bitmapfont/v4 v4.2.0)
- [x] Directory layout (`cmd/shooter`, `internal/*`)
- [x] `Taskfile.yml` (build / test / fmt / vet / check / run)
- [x] `git init`

## Phase 2 — Interfaces & data structures (no rendering)
- [x] `internal/doc` — `Document` interface, line-buffer model, edit ops,
      undo/redo command stack
- [x] `internal/events` — `Bus` interface + synchronous pub/sub
- [x] `internal/agent` — gunman entity + animation FSM
      (Idle/Walk/Aim/Fire/Recoil/Drag/Win)
- [x] `internal/actions` — FIFO command queue + projectiles (letters,
      bullets), line-swap animations, FX emission
- [x] `internal/grid` — shared cell<->pixel mapping (split out of render
      so actions and render can both use it without a cycle)
- [x] `internal/render` — `Layout` (camera + screen mapping), procedural
      pixel-art sprite sheet, particles, HUD
- [x] `internal/fileio` — `Store` interface, load/save `.txt`
- [x] `internal/ui` — `Editor` input mapping, service locator
      (`Services`), save/reload, help overlay

## Phase 3 — Tests (interfaces mocked)
- [x] doc: insert/delete/move-line/undo/redo/edge cases (26 tests)
- [x] events: subscribe/publish/unsubscribe (incl. mid-dispatch cancel)
- [x] agent: state transitions, walk completion, fire sequencing, drag
- [x] actions: typing queue, shot lifecycle, line moves, undo/redo,
      backpressure, status expiry, FX draining
- [x] ui: every key/mouse/ctrl mapping against a mock engine + fake store
- [x] render: layout geometry, camera clamping/easing, sprite parsing,
      pose mapping
- [x] grid/fileio: mapping round-trips, disk round-trips
- Total: 94 passing tests (`go test ./...`)

## Phase 4 — Implementation & wiring
- [x] Sprite frames (idle, 4-pose walk, aim, win) as ASCII pixel art with
      a validated palette parser
- [x] Ebitengine `Game` (Update/Draw/Layout) in `cmd/shooter`, fixed
      960x576 internal resolution
- [x] HUD (file + dirty marker, Ln/Col, status, key hints) + help overlay
      (F1)
- [x] CLI arg for file path; default `untitled.txt`
- [x] `-dump file.png` debug flag (renders 90 frames, saves a screenshot,
      exits) used to verify the renderer

## Phase 5 — Polish & verification
- [x] `gofmt` + `go vet` clean (wired into Taskfile before every build)
- [x] `go test ./...` green (94 tests)
- [x] Frame-dump visual verification: layout, text grid, sprite, HUD all
      checked pixel-wise; two real bugs found and fixed (GeoM call order,
      missing world->screen transform for entities)
- [x] User feedback "make him big": sprite redrawn at 12x16 art drawn 2x
      (24x32 screen: one row tall, two cells wide, font-matching pixels);
      column-pixel analysis verified every art row renders correctly
- [ ] Manual run by user: type, hold-backspace, move line, save/load,
      help overlay, click-to-walk

## Phase 7 — Session 2: Duke-ify (user feedback round 2)
- [x] Art: full redraw as Duke Nukem (platinum flat-top, black shades,
      red tank, gold buckle, steel-toed boots) with shading palette
- [x] Agent: new `Punch` melee state + `EvPunched` event (hands up close)
- [x] Actions: proximity weapon choice — Chebyshev distance <= 1 cell =
      hands (punch, no bullet), farther = gun (distance-based flight time)
- [x] Actions: `ShootAt(pos)` for explicit targets; caret stays put
      ("he shoots from that position")
- [x] UI/shell: right-click a glyph = shoot/punch it; left-click = walk
- [x] Line drag uses the hands pose (not the gun)
- [x] Tests for punch FSM, melee-vs-gun selection, ShootAt, right-click
- [x] Regenerate screenshot + SPEC/README updates

## Phase 6 — Docs & commit
- [x] `SPEC.md` (SRS with architecture diagrams)
- [x] `README.md` (quickstart, controls, build)
- [ ] Final `git commit` (done when the session is marked)

## Phase 8 — Session 3: word shotgun + sound + bigger head
- [x] Art: bigger head (7 rows: tall flat-top, thick 2-row glasses,
      wider jaw), restructured torso, new shotgun pose with wood forend,
      palette entry for wood
- [x] Agent: `Weapon` (pistol/shotgun) in Aim + Snapshot for pose choice
- [x] Actions: `ShootWord(back)` with in-line word ranges;
      Ctrl+Backspace/Ctrl+Delete bindings (UI + shell + mock + tests)
- [x] Actions: sound cues on View (slash/pistol/shotgun/rocket/stamp/save) +
      shotgun blast FX
- [x] `internal/audio`: procedural PCM synth (whoosh/crack/boom/launch/tick/chime),
      player pool, Ctrl+M mute; pure synth functions unit-tested
- [x] Shell: play View sounds each frame; Ctrl+M mute toggle
- [x] Verify art via frame dump; regenerate screenshot
- [x] Docs: SPEC/README/help/hints

## Phase 9 — Session 3b: katana, rockets, license, cross-builds (user requests)
- [x] Backspace = giant katana swing (rendered vector blade + arc trail,
      reach 4 cells, pistol beyond); slash state/EvSlash rename
- [x] Shotgun for words: Ctrl+Backspace / Ctrl+Delete (word ranges,
      pellet spread FX)
- [x] Rocket launcher for lines: Ctrl+K (emacs kill-line) and Ctrl+U
      (shell kill-to-start); frozen ranges, explosion FX
- [x] License: PolyForm Noncommercial 1.0.0 with git-config copyright
      (Xyzzy Apps), README + SPEC sections
- [x] Cross-compilation: task dist-{linux,darwin,darwin-arm64} (pure Go,
      CGO_ENABLED=0), all three verified to build
- [x] Sound: slash/pistol/shotgun/rocket/stamp/chime cues wired to View

## Phase 10 — Session 4: LAN multiplayer + chat bubbles + test bot (APPROVED)
- [x] `internal/netplay`: newline-delimited JSON protocol (Msg/DocOp/Peer),
      encode/decode + roundtrip tests
- [x] `internal/netplay`: ObservedDoc wrapper (emits DocOps, suppresses
      replay) + tests
- [x] `internal/netplay`: Host (TCP listen/broadcast, seq assignment) and
      Client (dial, in/out channels) + localhost integration test
- [x] Shell: `-serve addr` / `-join addr` / `-name` / `-bot` flags,
      message pump, presence throttle, quit cleanup
- [x] Undo/redo disabled while connected (status hint)
- [x] Chat: F2 draft line, Enter sends, Esc cancels; bubble rendering
- [x] Render: remote gunmen + name tags + cartoon chat bubbles (local too)
- [x] `internal/bot`: deterministic decision fn + cheesy lines + test;
      `-bot` joins and randomly walks/shoots/types/trash-talks
- [x] Docs + screenshot + task check
- [x] E2E verified on localhost: host + bot joined, snapshot synced,
      bot rockets/pistols fired, chat relayed ("walk it off", "too slow",
      "pwned"), leave detected on disconnect
## Phase 11 — Session 4b: menus and dialogs (user request)
- [x] Menu bar (File / Help) in the top HUD bar with shared hit-test
      geometry (internal/render/menu.go) + hover highlight (MouseMoved)
- [x] File menu: New (Ctrl+N too), Save, Close (dirty = double-confirm,
      then QuitRequested -> clean exit via errQuit)
- [x] Help dialog (F1/menu/Esc/click) and About dialog with the Gumroad
      support link + PolyForm license/copyright lines
- [x] Tests: menu open/dismiss, New/Save/Close paths, quit event,
      dirty-close confirmation, About content, hover, Esc handling
- [x] Screenshot + README/SPEC sync

## Phase 12 — Session 5: sample audio (user direction)
- [x] Default engine loads WAV samples from sounds/ (slash, pistol,
      shotgun, rocket, stamp, save); missing files = silent, device never
      opened; loaded count reported via Mode()
- [x] Procedural synth kept as the optional future engine behind -synth
- [x] Audio hidden from the UI: mute removed from help/hint bars
      (Ctrl+M still works, documented in SPEC only)
- [x] Tests: missing-dir silence, WAV fixture loading, mode labels

## Phase 13 — Session 6: ten-point feedback batch
- [x] (6) BUG: Ctrl+K/U never reach the editor (KeyK/KeyU/A/E/T/W added to watchedKeys) - KeyK/KeyU (and new
      chords A/E/T/W) missing from watchedKeys; add + tests
- [x] (4) Ctrl+A = beginning of line, (5) Ctrl+E = end of line (emacs)
- [x] (8) Glasses redraw: two separate 2x2 lens squares with a skin
      bridge (current full band reads as one big square)
- [x] (2) Rename product to DUKE (cmd/duke, bin/duke*, titles, docs): window title, About, help, README,
      binary names (module path stays shooter)
- [x] (3) Clickable About link opens the browser (shared dialog geometry) (GOOS switch:
      rundll32/open/xdg-open); Services.OpenURL + render hit helper + tests
- [x] (7) Audible audio: cmd/gensounds generator, commit six WAV
      samples into sounds/ so the default sample engine plays them
- [x] (9) README: how to add your own sprites (ASCII art format/palette)
- [x] (10) README: Gumroad link
- [x] (1) MULTIPLE TABS: per-tab {doc, caret, path}, tab bar UI,
      Ctrl+T/Ctrl+W, engine.SwapDoc, SetDocument/SetPath on the editor,
      LAN session stays bound to tab 1; tests
- [x] Final check + screenshot + docs sync

## Phase 14 — Session 7: real sound samples (user request)
- [x] Sounds replaced: extracted sample-accurate WAV cues from Sprite
      Fusion's destroy.spritefusion.com chip.flac atlas (pistol, shotgun,
      rocket, starSweep=katana, hitPaper=stamp, complete=save)
- [x] scripts/fetch-sfx.ps1 committed for regeneration / set swapping
- [x] sounds/_src gitignored; README credits Sprite Fusion
- [x] App verified loading all six samples
- [ ] Clarify item "Save to a file" (asked user: Save As feature or a
      save bug?)

## Phase 15 — Session 8: save-to-file + sound audition (user requests)
- [x] File > Save / Ctrl+S saves the ACTIVE TAB: untitled buffers open
      the native picker (Windows GetSaveFileNameW / macOS osascript /
      zenity+kdialog / working-dir fallback), picked path retargets the
      tab; cancel keeps dirty + no celebration; named buffers save in
      place. Tests: picker flow, cancel, named-no-dialog, isUntitled
- [x] -playsounds audition mode cycles all six cues (audio.Cues export,
      fx.Sound String for logs)
- [x] Cross-compile verified for all three picker implementations

## Phase 16 — Session 9: no ambient sound (user decision)
- [x] Removed the -playsounds audition flag and ticker entirely; sound
      now plays ONLY on editor actions (letters, weapons, save)
- [x] Removed the audio.Cues export; README audition line dropped
- [x] Verified: a plain 4s run emits zero play lines

## Phase 17 — Session 10: Duke mouth fix (user feedback)
- [x] Mouth (two pixels, one row) no longer touches the red tank: stole
      the hair-narrow row for a plain chin, so the face now reads
      cheek -> mouth -> chin; win frame restructured to match
- [x] Column-probe verified: cheek(art5), mouth(art6), chin(art7),
      tank(art8); render tests + build green

## Phase 19 - Session 12: direction-reflecting orientation + simple mouth
- [x] Facing now follows direction of travel/attack (supersedes the
      always-right rule): walking flips with dx, backspace faces the
      struck glyph, right-click faces the clicked X direction, heavy
      weapons face their aim (Ctrl+U left, Ctrl+K right), teleport
      resets to right; startShoot/startSpecial compute face from target
- [x] Mouth simplified to two dark pixels in one straight row
      (..ksskkssk.., win frame matched); pinned by a test
- [x] Tests rewritten to the new rule (171 total)

## Phase 20 - Session 13: clarity, UTF-8, machine gun, sword from the back
### (user approved: Scale 3 + bundled Go Mono, no downloads)
- [x] Bump render.Scale 2 -> 3 and agent.SpriteW/H 24x32 -> 36x48 (world
      px); grid stays art-driven (cellW = artW/2*Scale, cellH = artH*Scale)
      so the sprite still fills exactly two cells wide x one tall
- [x] Replace the bitmap font with an embedded TrueType monospace font
      (golang.org/x/image/font/gofont/gomono - already in go.sum, zero
      downloads); document face sized so a glyph advance == cellW exactly
      (columns stay aligned), 16px chrome face for HUD/help/tabs
- [x] Re-derive HUD/menu/dialog/tab geometry from the new face metrics
      instead of hardcoded 6px advances; layout/menu/geometry tests
      updated for the new window (1440x860)
- [x] UTF-8: doc/actions were already rune-based; new tests pin Go Mono
      column alignment for ASCII, accents, Greek, Cyrillic, CJK
      placeholders and combining marks; rendered dump verified
- [x] Machine gun: reworked the hip gun and aim frame into an SMG (long
      barrel, receiver, curved magazine) + sheathed katana on the back;
      help lines say machine gun; internals keep the Pistol name/sample
- [x] Sword from the back: fixed the stale recoil offset (x = -1 -> +1;
      the base art is left-native); katana pivot now starts at the back
      sheath and slides into the front hands, sweeping over the shoulder
      into the struck glyph (addresses "closer to the eyes"); verified
      pixel-exact for both facings via ASCII-frame dumps
- [x] Regenerate screenshot + SPEC/README (size, font, weapon art)
## Phase 20b - Session 13b: full art redraw (user feedback round)
- [x] Scale 3 made the 12x16 art blockier -> REDRAWN all frames at 18x24
      art pixels, Scale back to 2 (sprite stays 36x48 screen px, cell
      18x48, window 1440x860 - same layout, real detail)
- [x] Machine gun now VISIBLE at all times: idle/walk/aim all hold the
      SMG out front (6px barrel, receiver, magazine) Contra-style;
      verified via ASCII frame maps
- [x] Duke/Contra look: big flat-top, glinting 2-lens shades, shaded
      cheeks, buff red tank, gold buckle, jeans, steel-toed boots
- [x] Sword geometry rebuilt to add up: new frameKatanaDraw pose (he
      reaches BACK to the sheath); the blade pivot starts at the actual
      scabbard position (back edge) and slides to the front hands; the
      cut finishes pointing at the struck glyph at FACE height (was 22
      degrees down); katana stance keeps the empty scabbard on his back
- [x] All frames re-verified via ASCII maps; 174 tests green
- [ ] Final git commit (done when the session is marked)

## Phase 21 - Session 14: Sprite-Fusion stick hero + sprite sheet settings
- [x] Inspected destroy.spritefusion.com: its hero is procedural (canvas
      rig, not PNGs); extracted the rig proportions (head r7.5, torso 18,
      arms 12/12, thighs/shins 14/14, SMG anchors) from the JS bundle
- [x] Downloaded CC0 references (thekingphoenix/Bonsaiheldin gun hero,
      devurandom beat-em-up sheet) from OpenGameArt for style study
- [x] New "Stick" character sheet: 11 frames (idle/walk/aim/shotgun/
      rocket/katana/kdraw/win) at 18x24, round head + outline body + SMG
- [x] Renderer: sheet registry (duke/stick), SheetNames/ActiveSheet/
      SetSheet (unknown rejected), draw paths use the active sheet
- [x] Settings menu + dialog (top bar): lists sheets, marks the active
      one, click switches live; shell persists to settings.json and
      restores at launch; Settings button hover id + dropdown
- [x] Fixed a latent bug: the shell never wired Services.ToggleMute (the
      top-bar sound button was a silent no-op; Ctrl+M worked)
- [x] Tests: sheet switch/reject, settings menu/dialog, click-to-switch
      (177 total)
- [ ] Final git commit (done when the session is marked)

## Phase 22 - Session 15: image sprite sheets + JetBrains Mono
- [x] Font: embedded JetBrains Mono (SIL OFL 1.1, bundled) replaces Go
      Mono for both faces; column/advance tests kept green
- [x] Image sprite sheets (the site's strategy): sprites/<name>/<frame>.png
      folders; any size PNG fitted to the 18x24 art grid (nearest);
      per-pose fallback to the built-in duke art; new folders appear in
      Settings > Sprite Sheet and override same-named built-ins
- [x] cmd/gensprites exports the built-in Duke + Stick as editable PNGs
      (sprites/duke, sprites/stick); the game loads them from disk
- [x] Tests: disk loader + fallback + resize, sheet switch/reject (178)
- [x] Docs: README sprite-swap workflow + font credit; SPEC registry
- [ ] Final git commit (done when the session is marked)

## Phase 23 - Session 16: real character + uniform GUI font (user feedback)
- [x] BUG: Settings menu opened the FILE dropdown (drawMenu lacked the
      settings branch) - fixed via a shared MenuLabels helper
- [x] Deleted both hand-made sprites (duke PNGs + stick sheet); the
      character is now the CC0 gun-hero atlas by thekingphoenix/
      Bonsaiheldin, sliced and installed as sprites/duke (idle, 4-pose
      walk variants, SMG aim for all guns; katana poses reuse idle)
- [x] One font size for ALL GUI elements: dropped the 16px chrome face -
      single 30px JetBrains Mono face everywhere; HUD bars 48px, tab
      strip 44px, menus/dialogs re-measured; layout tests updated
- [x] Direction re-verified: dump shows the new character facing right
      with the SMG barrel forward; katana pivot tests still green
- [ ] Final git commit (done when the session is marked)

## Phase 24 - Session 17: "that's a girl" -> male trooper (user feedback)
- [x] The gun-hero atlas was tagged female (author: "no dude here") -
      replaced with Drakzlin's CC0 "Space soldier" (male trooper, rifle)
- [x] Sliced the 64px atlas: idle = standing rifle pose, walk = the
      4-frame rifle run cycle, aim/shotgun/rocket = the rifle attack
      pose, katana/kdraw/win = idle; mirrored to the left-native base
- [x] Verified via dump: trooper faces right with the rifle forward
- [ ] Final git commit (done when the session is marked)

## Phase 25 - Session 18: turn-first direction model (user direction)
- [x] The sprite owns a facing: default RIGHT (eyes toward the next
      character in the buffer); every directional key turns him FIRST
      and the NEXT press acts (backspace = turn left, then katana;
      arrows = turn, then walk; delete/word/line kills follow suit)
- [x] Engine: Step(dir) with line-wrap (EOL -> next line, col0 -> prev
      line end), turnIfNeeded(...) gate on Shoot/ShootWord/KillLine with
      FACING LEFT/RIGHT status; agent.Face(dir); ui arrows -> Step
- [x] Tests: turn-then-delete, turn-then-walk + wraps, already-facing
      acts immediately, arrow->Step routing (181 total)
- [x] Help dialog + README describe the model
- [ ] Final git commit (done when the session is marked)

## Phase 26 - Session 19: katana strike-facing fix + held right-click rapid fire
- [x] Bug: forward Delete targets the glyph AT the caret (target == base), and
      the old geometry rule ("own cell = left") flipped his facing mid-swing:
      facing right, the katana swung LEFT. Caret-derived attacks (backspace /
      delete / word / line kills) now strike the way the turn-first model
      faced him (back = left, forward = right); explicit right-clicks keep
      the geometric rule (face the clicked glyph; own-cell/vertical keep the
      current facing) - startShoot + startSpecial
- [x] Holding the right mouse button rapid-fires: the shell reissues the
      right-click at the pistol cadence (0.32s) while the button is held;
      the engine cooldown paces the actual shots
- [x] Tests: delete-on-own-cell slashes forward, delete-join slashes forward,
      explicit own-cell shot keeps the current facing; updated the old
      own-cell expectation (184 total)
- [x] Help dialog + README document the hold-to-rapid-fire
- [ ] Final git commit (done when the session is marked)

## Phase 27 - Session 20: no more dialog close hint, instant typing
- [x] Removed "Esc or click to close" from dialogs (Help/About/Sheet picker)
- [x] Typing no longer animates: the rune locks into the buffer instantly
      (no letter flight, no Stamp FX) - just the stamp sound; stripped the
      Letter machinery (engine, renderer, fx.Stamp, colLetter) and the
      old flight test rewritten (instant insert + SoundStamp + no FX)
- [x] Katana strike facing confirmed correct (delete/own-cell now strikes
      forward) - no further changes
- [x] Help dialog + README: instant typing, held right-click rapid fire
- [ ] Final git commit (done when the session is marked)
