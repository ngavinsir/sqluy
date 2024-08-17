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
	"github.com/ngavinsir/sqluy/fetcher"
	"github.com/ngavinsir/sqluy/keymap"
	"github.com/ngavinsir/sqluy/modal"
	"github.com/rivo/tview"
)

type (
	ShowModalArg struct {
		Refocus tview.Primitive
		Text    string
	}

	TabState struct {
		headers         []string
		rows            [][]string
		executionStart  time.Time
		executionFinish time.Time
		status          TabStatus
		query           string
	}

	State struct {
		tabStates   []TabState
		currentTab  uint8
		statusText  *tview.TextView
		modal       *modal.Modal
		currentView int
		views       []*tview.Box
		app         *tview.Application
	}
)

type TabStatus uint8

const (
	TabStatusEditing = iota
	TabStatusExecuting
)

//go:embed keymap.json
var keymapString string

func main() {
	var wg sync.WaitGroup
	app := tview.NewApplication()
	state := State{
		tabStates:  []TabState{{}},
		statusText: tview.NewTextView(),
		app:        app,
	}
	ctx, cancel := context.WithCancel(context.Background())
	km := keymap.New(keymapString)

	modalChan := make(chan ShowModalArg)
	delayDrawChan := make(chan time.Time)

	page := tview.NewPages()
	dataviewerPage := tview.NewPages()

	d := dataviewer.New(app, km)

	dataviewerModal := modal.NewModal().AddButtons([]string{"Cancel"}).SetBackgroundColor(tcell.ColorBlack)
	dataviewerModal.SetBorderColor(tcell.ColorBlack)
	dataviewerModal.Box.SetBackgroundColor(tcell.ColorBlack)
	state.modal = dataviewerModal

	dataviewerPage.AddPage("main", d, true, true)
	dataviewerPage.AddPage("modal", dataviewerModal, true, false)

	sqliteFetcher := fetcher.NewSqliteFetcher()

	flex := tview.NewFlex().SetDirection(tview.FlexRow)
	e := editor.New(
		editor.WithKeymapper(km),
		editor.WithApp(app),
		editor.WithDoneFunc(func(e *editor.Editor, s string) {
			if state.tabStates[state.currentTab].status != TabStatusEditing {
				return
			}
			state.tabStates[state.currentTab].executionStart = time.Now()
			state.tabStates[state.currentTab].status = TabStatusExecuting
			e.SetDisabled(true)
			dataviewerPage.ShowPage("modal")

			go func() {
				cols, rows, err := sqliteFetcher.Select(ctx, s)
				executionFinish := time.Now()

				app.QueueUpdateDraw(func() {
					if err != nil {
						modalChan <- ShowModalArg{Text: err.Error(), Refocus: flex}
					} else {
						d.SetData(cols, rows)
					}

					state.tabStates[state.currentTab].status = TabStatusEditing
					state.tabStates[state.currentTab].executionFinish = executionFinish
					state.FocusViewIndex(1)
					e.SetDisabled(false)
					dataviewerPage.HidePage("modal")
				})
			}()
		}),
	)
	e.SetViewModalFunc(func(text string) {
		modalChan <- ShowModalArg{Text: text, Refocus: e}
	})
	e.SetDelayDrawFunc(func(t time.Time) {
		delayDrawChan <- t
	})

	flex.
		AddItem(e, 0, 1, true).
		AddItem(dataviewerPage, 0, 1, false)

	modal := tview.NewModal().AddButtons([]string{"Ok"})

	page.AddPage("main", flex, true, true)
	page.AddPage("modal", modal, true, false)

	state.views = []*tview.Box{e.Box, d.Box}

	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyCtrlH {
			state.FocusViewIndex(state.currentView + 1)
			return nil
		}

		if event.Key() == tcell.KeyCtrlL {
			state.FocusViewIndex(state.currentView - 1)
			return nil
		}

		if event.Key() == tcell.KeyLF {
			return tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModCtrl)
		}
		return event
	})

	wg.Add(2)
	go modalLoop(ctx, modalChan, page, modal, app, &wg)
	go delayDrawLoop(ctx, &wg, delayDrawChan, app)
	go state.Draw(ctx, app)
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

func (s State) Draw(ctx context.Context, app *tview.Application) {
	t := time.NewTicker(10 * time.Millisecond)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			tabState := s.tabStates[s.currentTab]

			if tabState.executionStart.IsZero() {
				continue
			}

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

			app.QueueUpdateDraw(func() {
				s.modal.SetText(text)
			})
		}
	}
}

func (s *State) FocusViewIndex(index int) {
	if index < 0 {
		index = len(s.views) - 1
	}
	if index >= len(s.views) {
		index = 0
	}

	for i, box := range s.views {
		if i == index {
			s.app.SetFocus(box)
			s.currentView = i
			box.SetBorderColor(tcell.ColorWhite)
		} else {
			box.SetBorderColor(tcell.ColorGray)
		}
	}
}
