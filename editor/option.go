package editor

import "github.com/rivo/tview"

func WithApp(app *tview.Application) func(e *Editor) {
	return func(e *Editor) {
		e.app = app
	}
}

func WithKeymapper(km keymapper) func(e *Editor) {
	return func(e *Editor) {
		e.keymapper = km
	}
}

func WithDoneFunc(doneFn func(*Editor, string)) func(e *Editor) {
	return func(e *Editor) {
		e.onDoneFunc = doneFn
	}
}
