package painter

import (
	"image"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/internal/cache"
	"fyne.io/fyne/v2/theme"

	"github.com/goki/freetype/truetype"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

const (
	// DefaultTabWidth is the default width in spaces
	DefaultTabWidth = 4

	// TextDPI is a global constant that determines how text scales to interface sizes
	TextDPI = 78
)

// CachedFontFace returns a font face held in memory. These are loaded from the current theme.
func CachedFontFace(style fyne.TextStyle, opts *truetype.Options) font.Face {
	val, ok := fontCache.Load(style)
	if !ok {
		var f1, f2 *truetype.Font
		switch {
		case style.Monospace:
			f1 = loadFont(theme.TextMonospaceFont())
			f2 = loadFont(theme.DefaultTextMonospaceFont())
		case style.Bold:
			if style.Italic {
				f1 = loadFont(theme.TextBoldItalicFont())
				f2 = loadFont(theme.DefaultTextBoldItalicFont())
			} else {
				f1 = loadFont(theme.TextBoldFont())
				f2 = loadFont(theme.DefaultTextBoldFont())
			}
		case style.Italic:
			f1 = loadFont(theme.TextItalicFont())
			f2 = loadFont(theme.DefaultTextItalicFont())
		case style.Symbol:
			f2 = loadFont(theme.DefaultSymbolFont())
		default:
			f1 = loadFont(theme.TextFont())
			f2 = loadFont(theme.DefaultTextFont())
		}

		if f1 == nil {
			f1 = f2
		}
		val = &fontCacheItem{font: f1, fallback: f2, faces: make(map[truetype.Options]font.Face)}
		fontCache.Store(style, val)
	}

	comp := val.(*fontCacheItem)
	face := comp.faces[*opts]
	if face == nil {
		f1 := truetype.NewFace(comp.font, opts)
		f2 := truetype.NewFace(comp.fallback, opts)
		face = newFontWithFallback(f1, f2, comp.font, comp.fallback)

		comp.faces[*opts] = face
	}

	return face
}

// ClearFontCache is used to remove cached fonts in the case that we wish to re-load font faces
func ClearFontCache() {
	fontCache.Range(func(_, val interface{}) bool {
		item := val.(*fontCacheItem)
		for _, face := range item.faces {
			err := face.Close()

			if err != nil {
				fyne.LogError("failed to close font face", err)
				return false
			}
		}
		return true
	})

	fontCache = &sync.Map{}
}

// RenderedTextSize looks up how big a string would be if drawn on screen.
// It also returns the distance from top to the text baseline.
func RenderedTextSize(text string, fontSize float32, style fyne.TextStyle) (size fyne.Size, baseline float32) {
	size, base := cache.GetFontMetrics(text, fontSize, style)
	if base != 0 {
		return size, base
	}

	size, base = measureText(text, fontSize, style)
	cache.SetFontMetrics(text, fontSize, style, size, base)
	return size, base
}

func fixed266ToFloat32(i fixed.Int26_6) float32 {
	return float32(float64(i) / (1 << 6))
}

func loadFont(data fyne.Resource) *truetype.Font {
	loaded, err := truetype.Parse(data.Content())
	if err != nil {
		fyne.LogError("font load error", err)
	}

	return loaded
}

func measureText(text string, fontSize float32, style fyne.TextStyle) (fyne.Size, float32) {
	var opts truetype.Options
	opts.Size = float64(fontSize)
	opts.DPI = TextDPI

	face := CachedFontFace(style, &opts)
	advance := MeasureString(face, text, style.TabWidth)

	return fyne.NewSize(fixed266ToFloat32(advance), fixed266ToFloat32(face.Metrics().Height)),
		fixed266ToFloat32(face.Metrics().Ascent)
}

func newFontWithFallback(chosen, fallback font.Face, chosenFont, fallbackFont ttfFont) font.Face {
	return &compositeFace{chosen: chosen, fallback: fallback, chosenFont: chosenFont, fallbackFont: fallbackFont}
}

type compositeFace struct {
	sync.Mutex

	chosen, fallback         font.Face
	chosenFont, fallbackFont ttfFont
}

func (c *compositeFace) Close() (err error) {
	c.Lock()
	defer c.Unlock()

	if c.chosen != nil {
		err = c.chosen.Close()
	}

	err2 := c.fallback.Close()
	if err2 != nil {
		return err2
	}
	return
}

func (c *compositeFace) Glyph(dot fixed.Point26_6, r rune) (
	dr image.Rectangle, mask image.Image, maskp image.Point, advance fixed.Int26_6, ok bool) {
	c.Lock()
	defer c.Unlock()

	chosenContainsGlyph := c.containsGlyph(c.chosenFont, r)
	var fallbackContainsGlyph bool
	if !chosenContainsGlyph {
		fallbackContainsGlyph = c.containsGlyph(c.fallbackFont, r)
	}

	if chosenContainsGlyph {
		return c.chosen.Glyph(dot, r)
	}

	if fallbackContainsGlyph {
		return c.fallback.Glyph(dot, r)
	}

	return
}

func (c *compositeFace) GlyphAdvance(r rune) (advance fixed.Int26_6, ok bool) {
	c.Lock()
	defer c.Unlock()

	chosenContainsGlyph := c.containsGlyph(c.chosenFont, r)
	var fallbackContainsGlyph bool
	if !chosenContainsGlyph {
		fallbackContainsGlyph = c.containsGlyph(c.fallbackFont, r)
	}

	if chosenContainsGlyph {
		return c.chosen.GlyphAdvance(r)
	}

	if fallbackContainsGlyph {
		return c.fallback.GlyphAdvance(r)
	}

	return
}

func (c *compositeFace) GlyphBounds(r rune) (bounds fixed.Rectangle26_6, advance fixed.Int26_6, ok bool) {
	c.Lock()
	defer c.Unlock()

	chosenContainsGlyph := c.containsGlyph(c.chosenFont, r)
	var fallbackContainsGlyph bool
	if !chosenContainsGlyph {
		fallbackContainsGlyph = c.containsGlyph(c.fallbackFont, r)
	}

	if chosenContainsGlyph {
		return c.chosen.GlyphBounds(r)
	}

	if fallbackContainsGlyph {
		return c.fallback.GlyphBounds(r)
	}

	return
}

func (c *compositeFace) Kern(r0, r1 rune) fixed.Int26_6 {
	c.Lock()
	defer c.Unlock()

	contains0 := c.containsGlyph(c.chosenFont, r0)
	contains1 := c.containsGlyph(c.chosenFont, r1)

	if contains0 && contains1 {
		return c.chosen.Kern(r0, r1)
	}
	return c.fallback.Kern(r0, r1)
}

func (c *compositeFace) Metrics() font.Metrics {
	c.Lock()
	defer c.Unlock()

	return c.chosen.Metrics()
}

func (c *compositeFace) containsGlyph(font ttfFont, r rune) bool {
	return font != nil && font.Index(r) != 0
}

type ttfFont interface {
	Index(rune) truetype.Index
}

type fontCacheItem struct {
	font, fallback *truetype.Font
	faces          map[truetype.Options]font.Face
}

var fontCache = &sync.Map{} // map[fyne.TextStyle]*fontCacheItem
