package vim

var (
	AsyncMotion = [2]int{-23, -57}
)

func IsAsyncMotion(c [2]int) bool {
	return c == AsyncMotion
}
