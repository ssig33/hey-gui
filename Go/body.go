package main

import (
	"image"
	"image/color"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"gioui.org/x/richtext"
)

var (
	colorQuote    = color.NRGBA{R: 0x70, G: 0x70, B: 0x70, A: 0xff}
	colorQuoteBar = color.NRGBA{R: 0xcc, G: 0xcc, B: 0xcc, A: 0xff}
	colorCodeBg   = color.NRGBA{R: 0xf3, G: 0xf3, B: 0xf3, A: 0xff}
)

// renderedBody is a message body ready to lay out: rich blocks for Markdown and HTML,
// or selectable plain text.
type renderedBody struct {
	plain  bool
	text   string
	sel    widget.Selectable
	blocks []block
	states []richtext.InteractiveText
}

func renderBody(e *entry) *renderedBody {
	var blocks []block
	switch {
	case e.HTML != "" && isAttachmentOnly(e.Body):
		blocks = htmlBlocks(e.HTML)
	case looksLikeMarkdown(e.Body):
		blocks = markdownBlocks(e.Body)
	}
	if len(blocks) == 0 {
		return &renderedBody{plain: true, text: e.Body}
	}
	return &renderedBody{blocks: blocks, states: make([]richtext.InteractiveText, len(blocks))}
}

const bodySize = unit.Sp(13)

func (u *ui) bodyView(gtx layout.Context, rb *renderedBody, fg color.NRGBA) layout.Dimensions {
	if rb.plain {
		l := material.Label(u.th, bodySize, rb.text)
		l.State = &rb.sel
		l.LineHeightScale = 1.3
		l.Color = fg
		return l.Layout(gtx)
	}
	children := make([]layout.FlexChild, 0, len(rb.blocks)*2)
	for i := range rb.blocks {
		b, state := &rb.blocks[i], &rb.states[i]
		for {
			sp, ev, ok := state.Update(gtx)
			if !ok {
				break
			}
			if ev.Type == richtext.Click {
				if url, ok := sp.Get("url").(string); ok {
					openURL(url)
				}
			}
		}
		if i > 0 {
			gap := unit.Dp(8)
			if b.marker != "" || b.indent > 0 && rb.blocks[i-1].indent > 0 {
				gap = 3
			}
			children = append(children, layout.Rigid(layout.Spacer{Height: gap}.Layout))
		}
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.blockView(gtx, b, state, fg)
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (u *ui) blockView(gtx layout.Context, b *block, state *richtext.InteractiveText, fg color.NRGBA) layout.Dimensions {
	quoteW := gtx.Dp(12)
	left := b.quote * quoteW
	inset := layout.Inset{Left: unit.Dp(float32(left)/gtx.Metric.PxPerDp) + unit.Dp(18*max(b.indent-1, 0))}
	if b.indent > 0 {
		inset.Left += 18
	}
	content := func(gtx layout.Context) layout.Dimensions {
		switch b.kind {
		case blockRule:
			return layout.Inset{Top: 4, Bottom: 4}.Layout(gtx, hline)
		case blockCode:
			return layout.Background{}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					paint.FillShape(gtx.Ops, colorCodeBg, clip.Rect{Max: gtx.Constraints.Min}.Op())
					return layout.Dimensions{Size: gtx.Constraints.Min}
				},
				func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
					return layout.UniformInset(6).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return u.richText(gtx, b, state, fg, 12, true)
					})
				})
		case blockHeading:
			size := map[int]unit.Sp{1: 18, 2: 16, 3: 15}[b.level]
			if size == 0 {
				size = 14
			}
			return u.richText(gtx, b, state, fg, size, false)
		}
		return u.richText(gtx, b, state, fg, bodySize, false)
	}
	dims := inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if b.indent == 0 {
			return content(gtx)
		}
		// List items hang the marker to the left of the text.
		return layout.Flex{Alignment: layout.Baseline}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				w := gtx.Dp(18)
				gtx.Constraints.Min.X, gtx.Constraints.Max.X = w, w
				l := material.Label(u.th, bodySize, b.marker)
				l.Color = colorSecondary
				return l.Layout(gtx)
			}),
			layout.Flexed(1, content),
		)
	})
	// Draw one bar per quote level.
	for q := 0; q < b.quote; q++ {
		x := q*quoteW + gtx.Dp(2)
		paint.FillShape(gtx.Ops, colorQuoteBar, clip.Rect(image.Rect(x, 0, x+max(gtx.Dp(2), 1), dims.Size.Y)).Op())
	}
	return dims
}

func (u *ui) richText(gtx layout.Context, b *block, state *richtext.InteractiveText, fg color.NRGBA, size unit.Sp, mono bool) layout.Dimensions {
	styles := make([]richtext.SpanStyle, 0, len(b.spans))
	for _, s := range b.spans {
		st := richtext.SpanStyle{
			Font:    font.Font{Typeface: "Hiragino"},
			Size:    size,
			Color:   fg,
			Content: s.text,
		}
		if b.quote > 0 {
			st.Color = colorQuote
		}
		if s.bold || b.kind == blockHeading {
			st.Font.Weight = font.Bold
		}
		if s.italic {
			st.Font.Style = font.Italic
		}
		if s.code || mono {
			st.Font.Typeface = "Go Mono"
			st.Size = size - 1
		}
		if s.strike {
			st.Color = colorSecondary
		}
		if s.link != "" {
			st.Color = colorAccent
			st.Interactive = true
			st.Set("url", s.link)
		}
		styles = append(styles, st)
	}
	t := richtext.Text(state, u.th.Shaper, styles...)
	t.LineHeightScale = 1.3
	return t.Layout(gtx)
}
