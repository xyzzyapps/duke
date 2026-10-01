package render

import (
	"image/color"
	"math"
	"math/rand/v2"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"shooter/internal/fx"
)

// Particle colours.
var (
	colGold  = color.RGBA{255, 214, 90, 255}
	colWhite = color.RGBA{240, 246, 252, 255}
	colRed   = color.RGBA{235, 80, 80, 255}
	colGreen = color.RGBA{110, 220, 120, 255}
	colBlue  = color.RGBA{100, 160, 255, 255}
	colGray  = color.RGBA{140, 150, 160, 255}
	colFlash = color.RGBA{255, 244, 180, 255}
)

// particle is one short-lived world-space spark. When ch is non-zero the
// particle is drawn as a text glyph (destroyed letters fly as themselves).
type particle struct {
	x, y    float64
	vx, vy  float64
	grav    float64
	life    float64
	maxLife float64
	size    float64
	col     color.RGBA
	ch      rune
}

// spawnParticles converts one effect description into particles.
func spawnParticles(dst *[]particle, e fx.Effect) {
	switch e.Kind {
	case fx.Shatter:
		// The destroyed runes fly apart as glyphs themselves.
		n := 0
		for _, r := range e.Runes {
			if r == '\n' || r == '\t' || r == ' ' {
				continue
			}
			*dst = append(*dst, particle{
				x: e.X, y: e.Y,
				vx:   40 + rand.Float64()*70 - 35,
				vy:   -(60 + rand.Float64()*60),
				grav: 420,
				life: 0.75, maxLife: 0.75,
				size: 4, col: colWhite, ch: r,
			})
			n++
			if n >= 4 {
				break
			}
		}
		burst(dst, e.X, e.Y, 8, 110, 0.45, colGold, colGray, colWhite)
	case fx.Muzzle:
		burst(dst, e.X, e.Y, 5, 150, 0.14, colFlash, colGold)
	case fx.Blast:
		burst(dst, e.X, e.Y, 14, 230, 0.18, colFlash, colGold, colWhite)
	case fx.Explosion:
		burst(dst, e.X, e.Y, 24, 290, 0.55, colGold, colRed, colFlash)
		burst(dst, e.X, e.Y, 10, 80, 0.9, colGray) // smoke
	case fx.SaveFlare:
		burst(dst, e.X, e.Y, 16, 170, 0.95, colGold, colRed, colGreen, colBlue, colWhite)
	case fx.Puff:
		burst(dst, e.X, e.Y, 6, 45, 0.40, colGray)
	}
}

// burst appends n particles radiating from (x, y) with jittered speed and
// lifetime.
func burst(dst *[]particle, x, y float64, n int, speed, life float64, cols ...color.RGBA) {
	for i := 0; i < n; i++ {
		ang := rand.Float64() * 2 * math.Pi
		sp := speed * (0.4 + 0.6*rand.Float64())
		l := life * (0.6 + 0.7*rand.Float64())
		*dst = append(*dst, particle{
			x: x, y: y,
			vx:   math.Cos(ang) * sp,
			vy:   math.Sin(ang) * sp,
			grav: 160,
			life: l, maxLife: l,
			size: 2 + rand.Float64()*2.5,
			col:  cols[rand.IntN(len(cols))],
		})
	}
}

// updateParticles advances every particle and drops the dead ones.
func updateParticles(ps *[]particle, dt float64) {
	out := (*ps)[:0]
	for _, p := range *ps {
		p.life -= dt
		if p.life <= 0 {
			continue
		}
		p.vy += p.grav * dt
		p.x += p.vx * dt
		p.y += p.vy * dt
		out = append(out, p)
	}
	*ps = out
}

// drawParticles renders live particles in world space, offset into screen
// space. Glyph particles are handed to drawRune (which knows about the
// font); squares are plain rects.
func drawParticles(dst *ebiten.Image, ps []particle, offX, offY float64,
	drawRune func(r rune, x, y float64, col color.Color)) {
	for _, p := range ps {
		a := float64(p.life) / p.maxLife
		if a > 1 {
			a = 1
		}
		if p.ch != 0 {
			c := p.col
			c.A = uint8(float64(c.A) * a)
			drawRune(p.ch, p.x, p.y, c)
			continue
		}
		c := p.col
		c.A = uint8(float64(c.A) * a)
		s := float32(p.size)
		vector.DrawFilledRect(dst,
			float32(p.x+offX)-s/2, float32(p.y+offY)-s/2, s, s, c, false)
	}
}
