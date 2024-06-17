package main

import "github.com/rivo/tview"

func main() {
	flex := tview.NewFlex().
		AddItem(NewEditor(), 0, 1, false).
		AddItem(NewEditor(), 0, 1, true)
	if err := tview.NewApplication().SetRoot(flex, true).Run(); err != nil {
		panic(err)
	}
}
