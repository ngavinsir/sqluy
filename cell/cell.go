package cell

import "github.com/rivo/tview"

type (
	Cell struct {
		*tview.Box
		text string
	}
)
