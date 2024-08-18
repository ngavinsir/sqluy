package app

import (
	"context"
	_ "embed"
	"log"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/ngavinsir/sqluy/dataviewer"
	"github.com/ngavinsir/sqluy/editor"
	"github.com/ngavinsir/sqluy/fetcher"
	"github.com/ngavinsir/sqluy/keymap"
	"github.com/ngavinsir/sqluy/modal"
	"github.com/rivo/tview"
)

type (
	showModalArg struct {
		refocus tview.Primitive
		text    string
	}

	delayDrawArg struct {
		when time.Time
		fn   func()
	}

	tabState struct {
		headers         []string
		rows            [][]string
		executionStart  time.Time
		executionFinish time.Time
		status          TabStatus
		query           string
		ctx             context.Context
	}

	App struct {
		*tview.Pages
		ctx           context.Context
		app           *tview.Application
		tabStates     []*tabState
		currentTab    int
		statusText    *tview.TextView
		currentView   int
		views         []*tview.Box
		wg            *sync.WaitGroup
		delayDrawChan chan (delayDrawArg)
		showModalChan chan (showModalArg)
		mainModal     *tview.Modal
		focusDelegate func(tview.Primitive)
	}
)

type TabStatus uint8

const (
	TabStatusEditing = iota
	TabStatusExecuting
)

//go:embed keymap.json
var keymapString string

func New(ctx context.Context, wg *sync.WaitGroup, app *tview.Application) *App {
	km := keymap.New(keymapString)
	showModalChan := make(chan showModalArg)
	delayDrawChan := make(chan delayDrawArg)

	mainPage := tview.NewPages()
	dataviewerPage := tview.NewPages()

	a := App{
		wg:    wg,
		Pages: mainPage,
		tabStates: []*tabState{
			&tabState{
				ctx: context.Background(),
			},
		},
		statusText:    tview.NewTextView(),
		ctx:           ctx,
		app:           app,
		mainModal:     tview.NewModal().AddButtons([]string{"Ok"}),
		showModalChan: showModalChan,
		delayDrawChan: delayDrawChan,
	}

	d := dataviewer.New(km)

	dataviewerModal := modal.NewModal().AddButtons([]string{"Cancel"}).SetBackgroundColor(tcell.ColorBlack)
	dataviewerModal.SetBorderColor(tcell.ColorBlack)
	dataviewerModal.Box.SetBackgroundColor(tcell.ColorBlack)

	dataviewerPage.AddPage("main", d, true, true)
	dataviewerPage.AddPage("modal", dataviewerModal, true, false)

	sqliteFetcher := fetcher.NewSqliteFetcher()

	flex := tview.NewFlex().SetDirection(tview.FlexRow)
	e := editor.New(
		editor.WithKeymapper(km),
		editor.WithDoneFunc(func(e *editor.Editor, s string) {
			tabState := a.tabStates[a.currentTab]
			if tabState.status != TabStatusEditing {
				return
			}
			tabState.executionStart = time.Now()
			tabState.status = TabStatusExecuting
			e.SetDisabled(true)
			dataviewerPage.ShowPage("modal")

			go func() {
				cols, rows, err := sqliteFetcher.Select(tabState.ctx, s)
				executionFinish := time.Now()

				app.QueueUpdateDraw(func() {
					if err != nil {
						showModalChan <- showModalArg{text: err.Error(), refocus: flex}
					} else {
						d.SetData(cols, rows)
						if a.focusDelegate != nil {
							a.currentView = 1
							a.Focus(a.focusDelegate)
						}
					}

					tabState.status = TabStatusEditing
					tabState.executionFinish = executionFinish
					e.SetDisabled(false)
					dataviewerPage.HidePage("modal")
				})
			}()
		}),
	)
	e.SetViewModalFunc(func(text string) {
		showModalChan <- showModalArg{text: text, refocus: e}
	})
	e.SetDelayDrawFunc(func(t time.Time, fn func()) {
		delayDrawChan <- delayDrawArg{when: t, fn: fn}
	})

	flex.
		AddItem(e, 0, 1, true).
		AddItem(a.statusText, 1, 0, false).
		AddItem(dataviewerPage, 0, 1, false)

	mainPage.AddPage("main", flex, true, true)
	mainPage.AddPage("modal", a.mainModal, true, false)

	a.views = []*tview.Box{e.Box, d.Box}

	go a.modalLoop()
	go a.drawLoop()

	return &a
}

func (a *App) FocusViewIndex(index int) {
	if index < 0 {
		index = len(a.views) - 1
	}
	if index >= len(a.views) {
		index = 0
	}

	a.currentView = index
	a.app.SetFocus(a.views[index])
}

func (a *App) drawLoop() {
	a.wg.Add(1)
	defer a.wg.Done()

	t := time.NewTicker(10 * time.Millisecond)
	defer t.Stop()

	var args []delayDrawArg
	for {
		select {
		case <-a.ctx.Done():
			return
		case arg := <-a.delayDrawChan:
			args = append(args, arg)
			sort.Slice(args, func(i, j int) bool {
				return args[i].when.Before(args[j].when)
			})
		case <-t.C:
			if len(args) > 0 && args[0].when.After(time.Now()) {
				a.app.Draw()
				slices.DeleteFunc(args, func(arg delayDrawArg) bool {
					return arg.when.Before(time.Now())
				})
				return
			}

			tabState := a.tabStates[a.currentTab]

			if tabState.status == TabStatusExecuting {
				a.app.Draw()
			}
		}
	}
}

func (a *App) modalLoop() {
	a.wg.Add(1)
	defer a.wg.Done()

	for {
		select {
		case <-a.ctx.Done():
			return
		case arg := <-a.showModalChan:
			modalClosed := make(chan struct{})
			a.app.QueueUpdateDraw(func() {
				a.mainModal.SetText(arg.text).SetDoneFunc(func(buttonIndex int, buttonLabel string) {
					if buttonLabel == "Ok" {
						a.app.SetFocus(arg.refocus)
						a.Pages.HidePage("modal")
						modalClosed <- struct{}{}
					}
				})
				a.Pages.ShowPage("modal")
				a.app.SetFocus(a.mainModal)
			})

			select {
			case <-a.ctx.Done():
				return
			case <-modalClosed:
			}
		}
	}
}

func (a *App) Draw(screen tcell.Screen) {
	// draw views border color
	for i, view := range a.views {
		view.SetBorderColor(tcell.ColorGray)
		if i == a.currentView && view.HasFocus() {
			view.SetBorderColor(tcell.ColorWhite)
		}
	}

	a.Pages.Draw(screen)

	tabState := a.tabStates[a.currentTab]

	// draw status text
	if !tabState.executionStart.IsZero() {
		now := time.Now()
		if tabState.executionFinish.After(tabState.executionStart) {
			now = tabState.executionFinish
		}
		d := now.Sub(tabState.executionStart)
		durationText := d.Round(time.Millisecond).String()
		text := durationText
		if tabState.status == TabStatusExecuting {
			text = "executing... " + text
		}
		a.statusText.SetText(text)
		a.statusText.SetTextAlign(tview.AlignRight)
	}
}

func (a *App) Focus(delegate func(p tview.Primitive)) {
	a.focusDelegate = delegate
	a.Pages.Focus(delegate)
	a.FocusViewIndex(a.currentView)
}

func (a *App) Blur() {
	log.Println("blur")
	a.Pages.Blur()

	for _, box := range a.views {
		box.SetBorderColor(tcell.ColorGray)
	}
}

func (a *App) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return a.Pages.WrapInputHandler(func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		if event.Key() == tcell.KeyCtrlH {
			a.FocusViewIndex(a.currentView + 1)
			return
		}

		if event.Key() == tcell.KeyCtrlL {
			a.FocusViewIndex(a.currentView - 1)
			return
		}

		a.Pages.InputHandler()(event, setFocus)
	})
}
