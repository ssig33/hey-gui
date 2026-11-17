package main

import (
	"fmt"
	"image"
	"image/color"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/gesture"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

var (
	colorSecondary = color.NRGBA{R: 0x80, G: 0x80, B: 0x80, A: 0xff}
	colorAccent    = color.NRGBA{R: 0x1a, G: 0x6d, B: 0xe0, A: 0xff}
	colorSelected  = color.NRGBA{R: 0xd6, G: 0xe4, B: 0xfa, A: 0xff}
	colorSeparator = color.NRGBA{R: 0xe0, G: 0xe0, B: 0xe0, A: 0xff}
	colorError     = color.NRGBA{R: 0xd0, G: 0x20, B: 0x20, A: 0xff}
)

type ui struct {
	w   *app.Window
	th  *material.Theme
	m   *model
	cfg *config

	search     widget.Editor
	pathEditor widget.Editor
	reloadBtn  widget.Clickable
	sortBtn    widget.Clickable
	unreadBox  widget.Bool

	list         widget.List
	rowClicks    map[int64]*widget.Clickable
	lastSelected int64

	thread         widget.List
	threadFor      int64
	entryIndex     int
	threadViewport int
	openThread     widget.Clickable
	entryOpen      map[int64]*widget.Clickable
	htmlOpen       map[int64]*widget.Clickable

	divider        gesture.Drag
	dragStartAbs   float32
	dragStartWidth float32
}

func newUI(w *app.Window, th *material.Theme, m *model, cfg *config) *ui {
	u := &ui{w: w, th: th, m: m, cfg: cfg,
		rowClicks: map[int64]*widget.Clickable{}, entryOpen: map[int64]*widget.Clickable{}, htmlOpen: map[int64]*widget.Clickable{}}
	u.search.SingleLine = true
	u.search.Submit = true
	u.pathEditor.SingleLine = true
	u.pathEditor.SetText(cfg.HeyPath)
	u.list.Axis = layout.Vertical
	u.thread.Axis = layout.Vertical
	return u
}

func (u *ui) frame(gtx layout.Context) {
	paint.Fill(gtx.Ops, color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff})
	u.handleKeys(gtx)
	u.handleWidgets(gtx)

	if u.m.selected != u.lastSelected {
		u.lastSelected = u.m.selected
		u.scrollListToSelection()
	}

	layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(u.topBar),
		layout.Rigid(hline),
		layout.Flexed(1, u.main),
	)
}

func (u *ui) handleWidgets(gtx layout.Context) {
	for {
		ev, ok := u.search.Update(gtx)
		if !ok {
			break
		}
		switch ev.(type) {
		case widget.ChangeEvent:
			u.m.setQuery(u.search.Text())
		case widget.SubmitEvent:
			u.m.searchServer()
		}
	}
	if u.search.Text() != u.m.query {
		u.search.SetText(u.m.query)
	}
	if u.sortBtn.Clicked(gtx) {
		u.m.toggleSort()
	}
	if u.unreadBox.Update(gtx) {
		u.m.toggleUnreadOnly()
	}
	u.unreadBox.Value = u.cfg.UnreadOnly
	if u.reloadBtn.Clicked(gtx) {
		u.cfg.HeyPath = u.pathEditor.Text()
		u.cfg.save()
		u.m.reload()
	}
}

// handleKeys runs the single-key commands. While the search field has focus, keys go to it as text,
// and Esc clears the search.
func (u *ui) handleKeys(gtx layout.Context) {
	if gtx.Focused(&u.search) {
		for {
			ev, ok := gtx.Event(key.Filter{Focus: &u.search, Name: key.NameEscape})
			if !ok {
				break
			}
			if e, ok := ev.(key.Event); ok && e.State == key.Press {
				u.m.clearSearch()
				gtx.Execute(key.FocusCmd{})
			}
		}
		return
	}
	if gtx.Focused(&u.pathEditor) {
		return
	}
	shift := key.ModShift
	filters := []event.Filter{
		key.Filter{Name: "J", Optional: shift},
		key.Filter{Name: "K", Optional: shift},
		key.Filter{Name: "O", Optional: shift},
		key.Filter{Name: "G", Optional: shift},
		key.Filter{Name: "A", Required: shift},
		key.Filter{Name: "R"},
		key.Filter{Name: "U"},
		key.Filter{Name: "Q"},
		key.Filter{Name: "/"},
		key.Filter{Name: key.NameSpace, Optional: shift},
		key.Filter{Name: key.NameEscape},
		key.Filter{Name: key.NameReturn},
		key.Filter{Name: key.NameEnter},
		key.Filter{Name: key.NameDownArrow},
		key.Filter{Name: key.NameUpArrow},
	}
	for {
		ev, ok := gtx.Event(filters...)
		if !ok {
			break
		}
		e, ok := ev.(key.Event)
		if !ok || e.State != key.Press {
			continue
		}
		shifted := e.Modifiers.Contain(key.ModShift)
		switch e.Name {
		case "J":
			if shifted {
				u.moveEntry(1)
			} else {
				u.m.move(1)
			}
		case "K":
			if shifted {
				u.moveEntry(-1)
			} else {
				u.m.move(-1)
			}
		case key.NameDownArrow:
			u.m.move(1)
		case key.NameUpArrow:
			u.m.move(-1)
		case "O":
			if shifted {
				u.openSelected()
			} else {
				u.m.toggleSort()
			}
		case "G":
			if shifted {
				u.m.selectIndex(len(u.m.visible) - 1)
			} else {
				u.m.selectIndex(0)
			}
		case "A":
			u.m.markSeenAndAdvance()
		case "R":
			u.m.reload()
		case "U":
			u.m.toggleUnreadOnly()
		case "Q":
			u.w.Perform(system.ActionClose)
		case "/":
			gtx.Execute(key.FocusCmd{Tag: &u.search})
		case key.NameSpace:
			u.page(shifted)
		case key.NameEscape:
			u.m.clearSearch()
		case key.NameReturn, key.NameEnter:
			u.openSelected()
		}
	}
}

func (u *ui) openSelected() {
	if r, ok := u.m.selectedRow(); ok {
		openURL(r.appURL)
	}
}

func (u *ui) moveEntry(delta int) {
	n := len(u.m.threads[u.threadFor].entries)
	if n == 0 {
		return
	}
	u.entryIndex = min(max(u.entryIndex+delta, 0), n-1)
	// Item 0 of the thread list is the subject header.
	u.thread.Position = layout.Position{First: u.entryIndex + 1}
}

func (u *ui) page(up bool) {
	step := u.threadViewport * 9 / 10
	if up {
		step = -step
	}
	u.thread.Position.Offset += step
}

func (u *ui) scrollListToSelection() {
	i := u.m.indexOf(u.m.selected)
	pos := &u.list.Position
	if i < 0 {
		return
	}
	if i < pos.First {
		pos.First, pos.Offset = i, 0
	} else if pos.Count > 0 && i >= pos.First+pos.Count-1 {
		pos.First, pos.Offset = max(i-pos.Count+2, 0), 0
	}
}

// MARK: - Top bar

func (u *ui) topBar(gtx layout.Context) layout.Dimensions {
	m := u.m
	spacer := layout.Spacer{Width: 10}
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			w := gtx.Dp(280)
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = w, w
			return widget.Border{Color: colorSeparator, CornerRadius: 4, Width: 1}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: 4, Bottom: 4, Left: 6, Right: 6}.Layout(gtx, material.Editor(u.th, &u.search, T("Search (/)")).Layout)
			})
		}),
		layout.Rigid(spacer.Layout),
	}
	if m.searching {
		children = append(children, layout.Rigid(u.secondary("…")), layout.Rigid(spacer.Layout))
	}
	sortLabel := T("Newest first")
	if !u.cfg.NewestFirst {
		sortLabel = T("Oldest first")
	}
	children = append(children,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			b := material.Button(u.th, &u.sortBtn, sortLabel+" ▾")
			b.TextSize = 12
			b.Inset = layout.Inset{Top: 3, Bottom: 3, Left: 8, Right: 8}
			b.Background = color.NRGBA{R: 0xee, G: 0xee, B: 0xee, A: 0xff}
			b.Color = u.th.Fg
			return b.Layout(gtx)
		}),
		layout.Rigid(spacer.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			c := material.CheckBox(u.th, &u.unreadBox, T("Unread only (u)"))
			c.TextSize = 12
			c.IconColor = colorAccent
			return c.Layout(gtx)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: gtx.Constraints.Min} }),
	)
	if m.loadingRest {
		children = append(children, layout.Rigid(u.secondary(T("Loading the rest…"))), layout.Rigid(spacer.Layout))
	}
	children = append(children,
		layout.Rigid(u.label(T("Unread %d", len(m.unread)))),
		layout.Rigid(spacer.Layout),
		layout.Rigid(u.label(u.countLabel())),
		layout.Rigid(spacer.Layout),
		layout.Rigid(u.secondary(u.authLabel())),
	)
	return layout.Inset{Top: 6, Bottom: 6, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx, children...)
	})
}

func (u *ui) countLabel() string {
	m := u.m
	switch {
	case m.hasServer:
		return T("Server search: %d", len(m.visible))
	case m.query == "" && !u.cfg.UnreadOnly:
		return T("%d threads", len(m.rows))
	}
	return T("%d / %d threads", len(m.visible), len(m.rows))
}

func (u *ui) authLabel() string {
	switch u.m.auth {
	case authChecking:
		return T("Checking")
	case authLoggedIn:
		return T("Logged in")
	case authLoggedOut:
		return T("Logged out")
	case authCLIMissing:
		return T("hey not found")
	}
	return T("Error")
}

// MARK: - Main area

func (u *ui) main(gtx layout.Context) layout.Dimensions {
	m := u.m
	again := T("Press r to check again.")
	switch m.auth {
	case authChecking:
		return u.message(gtx, T("Checking hey CLI…"))
	case authCLIMissing:
		return u.cliMissing(gtx)
	case authLoggedOut:
		return u.message(gtx, T("Not logged in. Run `hey auth login` in Terminal."), again)
	case authError:
		return u.message(gtx, m.authErr, again)
	}
	if m.loading {
		return u.message(gtx, T("Loading Imbox…"))
	}
	var children []layout.FlexChild
	if m.loadErr != "" {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(6).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				l := material.Label(u.th, 11, m.loadErr)
				l.Color = colorError
				return l.Layout(gtx)
			})
		}), layout.Rigid(hline))
	}
	children = append(children, layout.Flexed(1, u.split))
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (u *ui) message(gtx layout.Context, lines ...string) layout.Dimensions {
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		var children []layout.FlexChild
		for _, line := range lines {
			children = append(children, layout.Rigid(u.secondary(line)), layout.Rigid(layout.Spacer{Height: 12}.Layout))
		}
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx, children...)
	})
}

func (u *ui) cliMissing(gtx layout.Context) layout.Dimensions {
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(u.secondary(T("hey command not found. Set its location below."))),
			layout.Rigid(layout.Spacer{Height: 12}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				w := gtx.Dp(420)
				gtx.Constraints.Min.X, gtx.Constraints.Max.X = w, w
				return widget.Border{Color: colorSeparator, CornerRadius: 4, Width: 1}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.UniformInset(6).Layout(gtx, material.Editor(u.th, &u.pathEditor, T("hey location (empty to search automatically)")).Layout)
				})
			}),
			layout.Rigid(u.secondary(T("Example: /opt/homebrew/bin/hey"))),
			layout.Rigid(layout.Spacer{Height: 12}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				b := material.Button(u.th, &u.reloadBtn, T("Reload with This Setting"))
				b.TextSize = 12
				return b.Layout(gtx)
			}),
		)
	})
}

// split lays out the list and the thread. The dragged list width is saved and restored on the next launch.
func (u *ui) split(gtx layout.Context) layout.Dimensions {
	total := gtx.Constraints.Max.X
	minL, minR := gtx.Dp(240), gtx.Dp(360)
	maxL := max(minL, total-minR)
	width := min(max(gtx.Dp(unit.Dp(u.cfg.ListWidth)), minL), maxL)

	for {
		e, ok := u.divider.Update(gtx.Metric, gtx.Source, gesture.Horizontal)
		if !ok {
			break
		}
		// Event positions are relative to the divider, which moves while dragging.
		abs := float32(width) + e.Position.X
		switch e.Kind {
		case pointer.Press:
			u.dragStartAbs, u.dragStartWidth = abs, float32(width)
		case pointer.Drag:
			width = min(max(int(u.dragStartWidth+abs-u.dragStartAbs), minL), maxL)
			u.cfg.ListWidth = float32(width) / gtx.Metric.PxPerDp
		case pointer.Release, pointer.Cancel:
			u.cfg.save()
		}
	}

	return layout.Flex{}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints = layout.Exact(image.Pt(width, gtx.Constraints.Max.Y))
			return u.postingList(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			size := image.Pt(gtx.Dp(5), gtx.Constraints.Max.Y)
			line := image.Rect(size.X/2, 0, size.X/2+max(gtx.Dp(1), 1), size.Y)
			paint.FillShape(gtx.Ops, colorSeparator, clip.Rect(line).Op())
			area := clip.Rect{Max: size}.Push(gtx.Ops)
			pointer.CursorColResize.Add(gtx.Ops)
			u.divider.Add(gtx.Ops)
			area.Pop()
			return layout.Dimensions{Size: size}
		}),
		layout.Flexed(1, u.threadPane),
	)
}

// MARK: - List

func (u *ui) postingList(gtx layout.Context) layout.Dimensions {
	rows := u.m.visible
	return material.List(u.th, &u.list).Layout(gtx, len(rows), func(gtx layout.Context, i int) layout.Dimensions {
		return u.postingRow(gtx, i, rows[i])
	})
}

func (u *ui) postingRow(gtx layout.Context, i int, r row) layout.Dimensions {
	c := u.rowClicks[r.topicID]
	if c == nil {
		c = new(widget.Clickable)
		u.rowClicks[r.topicID] = c
	}
	if c.Clicked(gtx) {
		u.m.selectIndex(i)
		u.lastSelected = u.m.selected
		gtx.Execute(key.FocusCmd{})
	}
	unread := u.m.unread[r.topicID]
	weight := font.Normal
	if unread {
		weight = font.Bold
	}
	return c.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				if r.topicID == u.m.selected {
					paint.FillShape(gtx.Ops, colorSelected, clip.Rect{Max: gtx.Constraints.Min}.Op())
				}
				bottom := image.Rect(0, gtx.Constraints.Min.Y-1, gtx.Constraints.Min.X, gtx.Constraints.Min.Y)
				paint.FillShape(gtx.Ops, colorSeparator, clip.Rect(bottom).Op())
				return layout.Dimensions{Size: gtx.Constraints.Min}
			},
			func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				return layout.Inset{Top: 3, Bottom: 3, Left: 4, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Alignment: layout.Baseline}.Layout(gtx,
								layout.Rigid(u.dot(unread)),
								layout.Rigid(u.secondary(shortDate(r.date))),
								layout.Rigid(layout.Spacer{Width: 6}.Layout),
								layout.Flexed(1, u.line(r.sender, weight, u.th.Fg)),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									if r.count <= 1 {
										return layout.Dimensions{}
									}
									return u.secondary(fmt.Sprintf("(%d)", r.count))(gtx)
								}),
							)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Left: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								text := r.subject
								return layout.Flex{Alignment: layout.Baseline}.Layout(gtx,
									layout.Rigid(u.line(text, weight, u.th.Fg)),
									layout.Rigid(layout.Spacer{Width: 8}.Layout),
									layout.Flexed(1, u.line(r.summary, font.Normal, colorSecondary)),
								)
							})
						}),
					)
				})
			},
		)
	})
}

func (u *ui) dot(unread bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		w := gtx.Dp(12)
		gtx.Constraints.Min.X, gtx.Constraints.Max.X = w, w
		if !unread {
			return layout.Dimensions{Size: image.Pt(w, 0)}
		}
		l := material.Label(u.th, 8, "●")
		l.Color = colorAccent
		return l.Layout(gtx)
	}
}

// MARK: - Thread

func (u *ui) threadPane(gtx layout.Context) layout.Dimensions {
	r, ok := u.m.selectedRow()
	if !ok {
		return layout.Center.Layout(gtx, u.secondary(T("Select a thread")))
	}
	if u.threadFor != r.topicID {
		u.threadFor = r.topicID
		u.thread.Position = layout.Position{}
		u.entryIndex = 0
	}
	if u.openThread.Clicked(gtx) {
		openURL(r.appURL)
	}
	u.threadViewport = gtx.Constraints.Max.Y
	t, loaded := u.m.threads[r.topicID]
	loaded = loaded && u.m.shown == r.topicID
	entries := t.entries
	n := 2
	if loaded {
		n = 1 + len(entries)
	}
	inset := layout.Inset{Left: 12, Right: 12}
	return material.List(u.th, &u.thread).Layout(gtx, n, func(gtx layout.Context, i int) layout.Dimensions {
		return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			switch {
			case i == 0:
				return u.threadHeader(gtx, r)
			case !loaded:
				if err := u.m.threadErrs[r.topicID]; err != "" {
					l := material.Label(u.th, 12, err)
					l.Color = colorError
					return l.Layout(gtx)
				}
				return u.secondary("…")(gtx)
			}
			return u.entryView(gtx, &entries[i-1])
		})
	})
}

func (u *ui) threadHeader(gtx layout.Context, r row) layout.Dimensions {
	return layout.Inset{Top: 12, Bottom: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Start}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				l := material.Label(u.th, 15, r.subject)
				l.Font.Weight = font.Bold
				return l.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: 8}.Layout),
			layout.Rigid(u.link(&u.openThread, T("Open in Browser"))),
		)
	})
}

func (u *ui) entryView(gtx layout.Context, e *entry) layout.Dimensions {
	open := u.entryOpen[e.ID]
	if open == nil {
		open = new(widget.Clickable)
		u.entryOpen[e.ID] = open
	}
	if open.Clicked(gtx) {
		openURL(e.webURL())
	}
	body := e.rendered
	if body == nil {
		body = renderBody(e)
		e.rendered = body
	}
	htmlOpen := u.htmlOpen[e.ID]
	if htmlOpen == nil {
		htmlOpen = new(widget.Clickable)
		u.htmlOpen[e.ID] = htmlOpen
	}
	if htmlOpen.Clicked(gtx) {
		openHTML(e.ID, e.HTML)
	}
	headers := []layout.FlexChild{layout.Rigid(u.header("From", e.sender(), true))}
	if e.Recipients != nil && len(e.Recipients.To) > 0 {
		headers = append(headers, layout.Rigid(u.header("To", joinContacts(e.Recipients.To), false)))
	}
	if e.Recipients != nil && len(e.Recipients.Cc) > 0 {
		headers = append(headers, layout.Rigid(u.header("Cc", joinContacts(e.Recipients.Cc), false)))
	}
	headers = append(headers, layout.Rigid(u.header("Date", longDate(parseTime(e.CreatedAt)), false)))
	if e.Kind != "" && e.Kind != "message" {
		headers = append(headers, layout.Rigid(u.header("Kind", e.Kind, false)))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(hline),
		layout.Rigid(layout.Spacer{Height: 6}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Start}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx, headers...)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					links := []layout.FlexChild{layout.Rigid(u.link(open, T("Open This Email")))}
					if e.HTML != "" {
						links = append(links, layout.Rigid(u.link(htmlOpen, T("Open HTML"))))
					}
					return layout.Flex{Axis: layout.Vertical, Alignment: layout.End}.Layout(gtx, links...)
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 8, Bottom: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				c := u.th.Fg
				if e.Kind != "" && e.Kind != "message" {
					c = colorSecondary
				}
				return u.bodyView(gtx, body, c)
			})
		}),
	)
}

func (u *ui) header(name, value string, bold bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Baseline}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				w := gtx.Dp(40)
				gtx.Constraints.Min.X, gtx.Constraints.Max.X = w, w
				l := material.Label(u.th, 11, name)
				l.Color = colorSecondary
				return l.Layout(gtx)
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				l := material.Label(u.th, 11, value)
				if bold {
					l.Font.Weight = font.Bold
				}
				return l.Layout(gtx)
			}),
		)
	}
}

func (u *ui) link(c *widget.Clickable, label string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return c.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			l := material.Label(u.th, 11, "↗ "+label)
			l.Color = colorAccent
			return l.Layout(gtx)
		})
	}
}

// MARK: - Small widgets

func (u *ui) label(text string) layout.Widget {
	return material.Label(u.th, 12, text).Layout
}

func (u *ui) secondary(text string) layout.Widget {
	l := material.Label(u.th, 12, text)
	l.Color = colorSecondary
	return l.Layout
}

// line is a single truncated line of text.
func (u *ui) line(text string, weight font.Weight, c color.NRGBA) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		l := material.Label(u.th, 12, text)
		l.MaxLines = 1
		l.Font.Weight = weight
		l.Color = c
		return l.Layout(gtx)
	}
}

func hline(gtx layout.Context) layout.Dimensions {
	size := image.Pt(gtx.Constraints.Max.X, max(gtx.Dp(1), 1))
	paint.FillShape(gtx.Ops, colorSeparator, clip.Rect{Max: size}.Op())
	return layout.Dimensions{Size: size}
}
