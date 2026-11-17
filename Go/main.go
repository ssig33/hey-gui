package main

import (
	"image"
	"log"
	"os"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/font/gofont"
	"gioui.org/font/opentype"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

func main() {
	cfg := loadConfig()
	go func() {
		w := new(app.Window)
		w.Option(
			app.Title("HEY Imbox"),
			app.Size(unit.Dp(cfg.WindowW), unit.Dp(cfg.WindowH)),
			app.MinSize(700, 400),
		)
		if err := run(w, cfg); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	app.Main()
}

func run(w *app.Window, cfg *config) error {
	posts := make(chan func(), 1024)
	post := func(f func()) {
		posts <- f
		w.Invalidate()
	}
	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.WithCollection(loadFonts()))
	th.Face = "Hiragino"
	th.TextSize = 12

	m := newModel(cfg, post)
	u := newUI(w, th, m, cfg)
	m.reload()

	var ops op.Ops
	var size image.Point
	var pxPerDp float32
	for {
		switch e := w.Event().(type) {
		case app.DestroyEvent:
			// Save the window size so the next launch opens at the same size.
			if pxPerDp > 0 && size.X > 0 {
				cfg.WindowW = float32(size.X) / pxPerDp
				cfg.WindowH = float32(size.Y) / pxPerDp
			}
			cfg.save()
			return e.Err
		case app.ConfigEvent:
			size = e.Config.Size
		case app.FrameEvent:
		drain:
			for {
				select {
				case f := <-posts:
					f()
				default:
					break drain
				}
			}
			gtx := app.NewContext(&ops, e)
			pxPerDp = gtx.Metric.PxPerDp
			u.frame(gtx)
			e.Frame(gtx.Ops)
		}
	}
}

// loadFonts uses Hiragino Sans for Japanese text, with the Go fonts as a fallback.
func loadFonts() []font.FontFace {
	var faces []font.FontFace
	for _, f := range []struct {
		path   string
		weight font.Weight
	}{
		{"/System/Library/Fonts/ヒラギノ角ゴシック W3.ttc", font.Normal},
		{"/System/Library/Fonts/ヒラギノ角ゴシック W6.ttc", font.Bold},
	} {
		data, err := os.ReadFile(f.path)
		if err != nil {
			continue
		}
		coll, err := opentype.ParseCollection(data)
		if err != nil || len(coll) == 0 {
			continue
		}
		face := coll[0]
		face.Font.Typeface = "Hiragino"
		face.Font.Weight = f.weight
		faces = append(faces, face)
	}
	return append(faces, gofont.Collection()...)
}
