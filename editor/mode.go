package editor

type mode uint8

const (
	normal mode = iota
	insert
	replace
)

func (m mode) String() string {
	switch m {
	case insert:
		return "INSERT"
	case replace:
		return "REPLACE"
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
	default:
		return "n"
	}
}
