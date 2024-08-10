package main

import (
	"context"
	_ "embed"
	"sort"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/ngavinsir/sqluy/dataviewer"
	"github.com/ngavinsir/sqluy/editor"
	"github.com/ngavinsir/sqluy/flex"
	"github.com/ngavinsir/sqluy/keymap"
	"github.com/rivo/tview"
)

type (
	ShowModalArg struct {
		Refocus tview.Primitive
		Text    string
	}
)

//go:embed keymap.json
var keymapString string

func main() {
	var wg sync.WaitGroup
	app := tview.NewApplication()
	ctx, cancel := context.WithCancel(context.Background())
	km := keymap.New(keymapString)

	modalChan := make(chan ShowModalArg)
	delayDrawChan := make(chan time.Time)

	page := tview.NewPages()

	e := editor.New(km, app)
	e.SetViewModalFunc(func(text string) {
		modalChan <- ShowModalArg{Text: text, Refocus: e}
	})
	e.SetDelayDrawFunc(func(t time.Time) {
		delayDrawChan <- t
	})

	d := dataviewer.New(app, km)

	flex := flex.New().SetDirection(tview.FlexRow).
		AddItem(e, 0, 1, false).
		AddItem(d, 0, 1, true)

	modal := tview.NewModal().AddButtons([]string{"Ok"})
	modalFlex := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(modal, 0, 1, false).
			AddItem(nil, 0, 1, false),
			0, 1, false).
		AddItem(nil, 0, 1, false)

	page.AddPage("main", flex, true, true)
	page.AddPage("modal", modalFlex, true, false)
	page.SetRect(0, 0, 31, 27)

	wg.Add(2)
	go modalLoop(ctx, modalChan, page, modal, app, &wg)
	go delayDrawLoop(ctx, &wg, delayDrawChan, app)
	err := app.SetRoot(page, true).Run()
	cancel()
	wg.Wait()

	if err != nil {
		panic(err)
	}
}

func delayDrawLoop(ctx context.Context, wg *sync.WaitGroup, c chan time.Time, app *tview.Application) {
	defer wg.Done()

	var times []time.Time
	for {
		sort.Slice(times, func(i, j int) bool {
			return times[i].Before(times[j])
		})
		t := time.NewTimer(100 * time.Hour)
		if len(times) > 0 {
			t.Stop()
			t = time.NewTimer(time.Until(times[0]))
		}
		defer t.Stop()
		select {
		case <-ctx.Done():
			return
		case t := <-c:
			times = append(times, t)
		case <-t.C:
			times = times[1:]
			app.QueueEvent(tcell.NewEventKey(tcell.KeyEsc, 0, tcell.ModNone))
		}
	}
}

func modalLoop(ctx context.Context, c chan ShowModalArg, p *tview.Pages, m *tview.Modal, app *tview.Application, wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case arg := <-c:
			modalClosed := make(chan struct{})
			app.QueueUpdateDraw(func() {
				m.SetText(arg.Text).SetDoneFunc(func(buttonIndex int, buttonLabel string) {
					if buttonLabel == "Ok" {
						app.SetFocus(arg.Refocus)
						p.HidePage("modal")
						modalClosed <- struct{}{}
					}
				})
				p.ShowPage("modal")
				app.SetFocus(m)
			})

			select {
			case <-ctx.Done():
				return
			case <-modalClosed:
			}
		}
	}
}
