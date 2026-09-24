package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// ── toggleSwitch — скользящий тумблер как в макете ──
type toggleSwitch struct {
	widget.BaseWidget
	On    bool
	OnTap func()
}

func newToggleSwitch() *toggleSwitch {
	t := &toggleSwitch{}
	t.ExtendBaseWidget(t)
	return t
}

func (t *toggleSwitch) Tapped(_ *fyne.PointEvent) {
	if t.OnTap != nil {
		t.OnTap()
	}
}

func (t *toggleSwitch) MinSize() fyne.Size { return fyne.NewSize(72, 38) }

func (t *toggleSwitch) CreateRenderer() fyne.WidgetRenderer {
	track := canvas.NewRectangle(color.NRGBA{0x2f, 0x39, 0x47, 0xff})
	track.CornerRadius = 19
	thumb := canvas.NewCircle(color.White)
	return &toggleRenderer{t: t, track: track, thumb: thumb}
}

type toggleRenderer struct {
	t     *toggleSwitch
	track *canvas.Rectangle
	thumb *canvas.Circle
}

func (r *toggleRenderer) Layout(s fyne.Size) {
	r.track.Resize(s)
	d := s.Height - 8
	r.thumb.Resize(fyne.NewSize(d, d))
	x := float32(4)
	if r.t.On {
		x = s.Width - d - 4
	}
	r.thumb.Move(fyne.NewPos(x, 4))
}
func (r *toggleRenderer) MinSize() fyne.Size { return fyne.NewSize(72, 38) }
func (r *toggleRenderer) Refresh() {
	if r.t.On {
		r.track.FillColor = color.NRGBA{0x5b, 0xa3, 0x2b, 0xff}
	} else {
		r.track.FillColor = color.NRGBA{0x2f, 0x39, 0x47, 0xff}
	}
	r.track.Refresh()
	r.Layout(r.t.Size())
	canvas.Refresh(r.t)
}
func (r *toggleRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.track, r.thumb} }
func (r *toggleRenderer) Destroy()                     {}

// ── tapCard — кликабельная карточка со скруглённым фоном (строки/сегменты) ──
type tapCard struct {
	widget.BaseWidget
	bg      *canvas.Rectangle
	content fyne.CanvasObject
	OnTap   func()
}

func newTapCard(content fyne.CanvasObject, fill color.Color) *tapCard {
	c := &tapCard{content: content}
	c.bg = canvas.NewRectangle(fill)
	c.bg.CornerRadius = 10
	c.ExtendBaseWidget(c)
	return c
}

func (c *tapCard) Tapped(_ *fyne.PointEvent) {
	if c.OnTap != nil {
		c.OnTap()
	}
}

func (c *tapCard) SetFill(col color.Color) {
	c.bg.FillColor = col
	c.bg.Refresh()
}

func (c *tapCard) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(container.NewStack(c.bg, container.NewPadded(c.content)))
}
