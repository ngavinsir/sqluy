package editor

type mode uint8

const (
	ModeNormal mode = iota
	ModeInsert
	ModeReplace
	ModeVisual
	ModeVLine
)

func (m mode) String() string {
	switch m {
	case ModeInsert:
		return "INSERT"
	case ModeReplace:
		return "REPLACE"
	case ModeVisual:
		return "VISUAL"
	case ModeVLine:
		return "V-LINE"
	default:
		return "NORMAL"
	}
}

func (m mode) ShortString() string {
	switch m {
	case ModeInsert:
		return "i"
	case ModeReplace:
		return "r"
	case ModeVisual, ModeVLine:
		return "v"
	default:
		return "n"
	}
}
