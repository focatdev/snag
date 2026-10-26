package winapi

import (
	"bytes"
	_ "embed"
	"image"
	"image/png"
	"math"
	"sync"
)

//go:embed hook.png
var hookPNG []byte

type ToastKind int

const (
	ToastSuccess ToastKind = iota
	ToastInfo
	ToastError
)

// dips
const (
	tPad    = 20.0
	tW      = 336.0
	tH      = 68.0
	tRadius = 12.0
	tTile   = 40.0
	tTileR  = 9.0
	tInset  = 14.0
)

type rgba struct{ r, g, b float32 }

func hexc(v uint32) rgba {
	return rgba{float32(v>>16&0xff) / 255, float32(v>>8&0xff) / 255, float32(v&0xff) / 255}
}

// app colour tokens
var (
	cRaised   = hexc(0x1a1c1f)
	cLine     = hexc(0x2a2d31)
	cFg       = hexc(0xe8ebef)
	cMuted    = hexc(0x8b9199)
	cFrost    = hexc(0xdde4ee)
	cFrostInk = hexc(0x151a21)
	cHover    = hexc(0x26292d)
	cBadTile  = hexc(0x3a2423)
	cBad      = hexc(0xf0857c)
)

type canvas struct {
	w, h int
	px   []float32 // premultiplied rgba
}

func newCanvas(w, h int) *canvas { return &canvas{w: w, h: h, px: make([]float32, w*h*4)} }

func (c *canvas) blend(x, y int, col rgba, a float32) {
	if a <= 0 || x < 0 || y < 0 || x >= c.w || y >= c.h {
		return
	}
	if a > 1 {
		a = 1
	}
	i := (y*c.w + x) * 4
	k := 1 - a
	c.px[i] = col.r*a + c.px[i]*k
	c.px[i+1] = col.g*a + c.px[i+1]*k
	c.px[i+2] = col.b*a + c.px[i+2]*k
	c.px[i+3] = a + c.px[i+3]*k
}

func boxDist(x, y, cx, cy, hw, hh, r float64) float64 {
	qx := math.Abs(x-cx) - (hw - r)
	qy := math.Abs(y-cy) - (hh - r)
	ox, oy := math.Max(qx, 0), math.Max(qy, 0)
	return math.Hypot(ox, oy) + math.Min(math.Max(qx, qy), 0) - r
}

func clamp01(v float64) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return float32(v)
}

func (c *canvas) roundRect(x0, y0, w, h, r float64, col rgba, alpha float32) {
	cx, cy := x0+w/2, y0+h/2
	for y := int(y0) - 1; y <= int(y0+h)+1; y++ {
		for x := int(x0) - 1; x <= int(x0+w)+1; x++ {
			d := boxDist(float64(x)+0.5, float64(y)+0.5, cx, cy, w/2, h/2, r)
			c.blend(x, y, col, clamp01(0.5-d)*alpha)
		}
	}
}

func (c *canvas) ring(x0, y0, w, h, r, width float64, col rgba) {
	cx, cy := x0+w/2, y0+h/2
	for y := int(y0) - 1; y <= int(y0+h)+1; y++ {
		for x := int(x0) - 1; x <= int(x0+w)+1; x++ {
			d := boxDist(float64(x)+0.5, float64(y)+0.5, cx, cy, w/2, h/2, r)
			outer := clamp01(0.5 - d)
			inner := clamp01(0.5 - (d + width))
			c.blend(x, y, col, outer-inner)
		}
	}
}

func (c *canvas) shadow(x0, y0, w, h, r, blur float64, alpha float32) {
	cx, cy := x0+w/2, y0+h/2
	black := rgba{}
	for y := 0; y < c.h; y++ {
		for x := 0; x < c.w; x++ {
			d := boxDist(float64(x)+0.5, float64(y)+0.5, cx, cy, w/2, h/2, r)
			if d > blur {
				continue
			}
			t := (d + blur*0.3) / (blur * 1.3)
			if t < 0 {
				t = 0
			}
			fall := 1 - t*t*(3-2*t)
			c.blend(x, y, black, float32(fall)*alpha)
		}
	}
}

func (c *canvas) mask(m []uint8, mw, mh, ox, oy int, col rgba) {
	for y := 0; y < mh; y++ {
		for x := 0; x < mw; x++ {
			if v := m[y*mw+x]; v > 0 {
				c.blend(ox+x, oy+y, col, float32(v)/255)
			}
		}
	}
}

var (
	hookOnce sync.Once
	hookSrc  []uint8
	hookN    int
)

func hookMask(size int) []uint8 {
	hookOnce.Do(func() {
		img, err := png.Decode(bytes.NewReader(hookPNG))
		if err != nil {
			return
		}
		b := img.Bounds()
		hookN = b.Dx()
		hookSrc = make([]uint8, hookN*hookN)
		for y := 0; y < hookN; y++ {
			for x := 0; x < hookN; x++ {
				switch g := img.(type) {
				case *image.Gray:
					hookSrc[y*hookN+x] = g.GrayAt(x+b.Min.X, y+b.Min.Y).Y
				default:
					_, _, _, a := img.At(x+b.Min.X, y+b.Min.Y).RGBA()
					r, _, _, _ := img.At(x+b.Min.X, y+b.Min.Y).RGBA()
					if a == 0xffff {
						a = r
					}
					hookSrc[y*hookN+x] = uint8(a >> 8)
				}
			}
		}
	})
	out := make([]uint8, size*size)
	if hookN == 0 || size <= 0 {
		return out
	}
	step := float64(hookN) / float64(size)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			x0, y0 := int(float64(x)*step), int(float64(y)*step)
			x1, y1 := int(float64(x+1)*step), int(float64(y+1)*step)
			if x1 <= x0 {
				x1 = x0 + 1
			}
			if y1 <= y0 {
				y1 = y0 + 1
			}
			sum, n := 0, 0
			for sy := y0; sy < y1 && sy < hookN; sy++ {
				for sx := x0; sx < x1 && sx < hookN; sx++ {
					sum += int(hookSrc[sy*hookN+sx])
					n++
				}
			}
			// thin strokes fade out when shrunk this much
			if n > 0 {
				out[y*size+x] = uint8(math.Min(255, float64(sum/n)*1.35))
			}
		}
	}
	return out
}

type toastView struct {
	kind     ToastKind
	title    string
	sub      string
	progress float64 // below zero hides the bar
}

type textFn func(s string, semibold bool, sizePx, w, h int) []uint8

func px2(v, scale float64) float64 { return math.Round(v * scale) }

// premultiplied bgra for UpdateLayeredWindow
func renderToast(v toastView, scale float64, text textFn) (w, h int, bgra []byte) {
	pad := px2(tPad, scale)
	cw, ch := px2(tW, scale), px2(tH, scale)
	w, h = int(cw+pad*2), int(ch+pad*2)
	c := newCanvas(w, h)
	r := px2(tRadius, scale)

	c.shadow(pad, pad+px2(6, scale), cw, ch, r, px2(18, scale), 0.42)
	c.roundRect(pad, pad, cw, ch, r, cRaised, 1)
	c.ring(pad, pad, cw, ch, r, math.Max(1, math.Round(scale)), cLine)

	tile := px2(tTile, scale)
	tx, ty := pad+px2(tInset, scale), pad+(ch-tile)/2
	tileCol, iconCol := cFrost, cFrostInk
	switch v.kind {
	case ToastInfo:
		tileCol, iconCol = cHover, cFrost
	case ToastError:
		tileCol, iconCol = cBadTile, cBad
	}
	c.roundRect(tx, ty, tile, tile, px2(tTileR, scale), tileCol, 1)
	if v.kind == ToastError {
		bang := int(px2(4, scale))
		c.roundRect(tx+tile/2-float64(bang)/2, ty+tile*0.24, float64(bang), tile*0.34, float64(bang)/2, iconCol, 1)
		c.roundRect(tx+tile/2-float64(bang)/2, ty+tile*0.66, float64(bang), float64(bang), float64(bang)/2, iconCol, 1)
	} else {
		is := int(px2(27, scale))
		c.mask(hookMask(is), is, is, int(tx+(tile-float64(is))/2), int(ty+(tile-float64(is))/2), iconCol)
	}

	textX := int(tx + tile + px2(12, scale))
	textW := int(pad+cw) - textX - int(px2(16, scale))
	lineH := int(px2(20, scale))
	top := int(pad + ch/2 - px2(19, scale))
	if v.progress >= 0 {
		top -= int(px2(3, scale))
	}
	if text != nil && textW > 0 {
		if m := text(v.title, true, int(px2(14, scale)), textW, lineH); m != nil {
			c.mask(m, textW, lineH, textX, top, cFg)
		}
		if m := text(v.sub, false, int(px2(12.5, scale)), textW, lineH); m != nil {
			c.mask(m, textW, lineH, textX, top+int(px2(18, scale)), cMuted)
		}
	}

	if v.progress >= 0 {
		bh := math.Max(2, px2(3, scale))
		by := float64(top) + px2(42, scale)
		c.roundRect(float64(textX), by, float64(textW), bh, bh/2, cLine, 1)
		p := math.Min(1, v.progress)
		if p > 0 {
			c.roundRect(float64(textX), by, math.Max(bh, float64(textW)*p), bh, bh/2, cFrost, 1)
		}
	}

	bgra = make([]byte, w*h*4)
	for i := 0; i < w*h; i++ {
		s := c.px[i*4 : i*4+4]
		bgra[i*4] = uint8(math.Round(float64(s[2]) * 255))
		bgra[i*4+1] = uint8(math.Round(float64(s[1]) * 255))
		bgra[i*4+2] = uint8(math.Round(float64(s[0]) * 255))
		bgra[i*4+3] = uint8(math.Round(float64(s[3]) * 255))
	}
	return w, h, bgra
}
