package main

import (
	"context"
	_ "embed"
	"sync"

	"github.com/ngavinsir/sqluy/editor"
	"github.com/ngavinsir/sqluy/keymap"
	"github.com/rivo/tview"
)

type (
	ShowModalArg struct {
		Wg      *sync.WaitGroup
		Refocus tview.Primitive
		Text    string
	}
)

//go:embed keymap.json
var keymapString string

func main() {
	km := keymap.New(keymapString)
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())

	modalChan := make(chan ShowModalArg)

	page := tview.NewPages()

	e := editor.New(false)
	e.SetViewModalFunc(func(text string) {
		var wg sync.WaitGroup
		modalChan <- ShowModalArg{Text: text, Wg: &wg, Refocus: e}
	})
	// flex := tview.NewFlex().
	// 	AddItem(e, 0, 1, true)
	modal := tview.NewModal().AddButtons([]string{"Ok"})
	modalFlex := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(modal, 0, 1, false).
			AddItem(nil, 0, 1, false),
			0, 1, false).
		AddItem(nil, 0, 1, false)

	page.AddPage("main", e, true, true)
	page.AddPage("modal", modalFlex, true, false)
	page.SetRect(0, 0, 15, 8)

	wg.Add(1)
	app := tview.NewApplication()
	go modalLoop(ctx, modalChan, page, modal, app, &wg)
	err := app.SetRoot(page, true).Run()
	cancel()
	wg.Wait()

	if err != nil {
		panic(err)
	}
}

func modalLoop(ctx context.Context, c chan ShowModalArg, p *tview.Pages, m *tview.Modal, app *tview.Application, wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case arg := <-c:
			arg.Wg.Add(1)
			app.QueueUpdateDraw(func() {
				m.SetText(arg.Text).SetDoneFunc(func(buttonIndex int, buttonLabel string) {
					if buttonLabel == "Ok" {
						app.SetFocus(arg.Refocus)
						p.HidePage("modal")
						arg.Wg.Done()
					}
				})
				p.ShowPage("modal")
				app.SetFocus(m)
			})

			c := make(chan struct{})
			go func() {
				defer close(c)
				arg.Wg.Wait()
			}()
			select {
			case <-ctx.Done():
				return
			case <-c:
			}
		}
	}
}
