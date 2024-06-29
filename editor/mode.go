package editor

type mode uint8

const (
	normal mode = iota
	insert
	replace
	visual
)

func (m mode) String() string {
	switch m {
	case insert:
		return "INSERT"
	case replace:
		return "REPLACE"
	case visual:
		return "VISUAL"
	default:
		return "NORMAL"
	}
}

func (m mode) ShortString() string {
	switch m {
	case insert:
		return "i"
	case replace:
		return "r"
	case visual:
		return "v"
	default:
		return "n"
	}
}
