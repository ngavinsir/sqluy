package main

import (
	"context"
	_ "embed"
	"sync"

	"github.com/gdamore/tcell/v2"
	"github.com/ngavinsir/sqluy/app"
	"github.com/rivo/tview"
)

func main() {
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())

	application := tview.NewApplication()
	a := app.New(ctx, &wg, application)

	application.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyLF {
			event = tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModCtrl)
		}
		return event
	})

	err := application.SetRoot(a, true).Run()
	cancel()
	wg.Wait()

	if err != nil {
		panic(err)
	}
}
