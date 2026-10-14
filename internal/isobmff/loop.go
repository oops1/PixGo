package isobmff

func PlayCount(loopCount int) int {
	switch {
	case loopCount == 0:
		return 0
	case loopCount < 0:
		return 1
	}
	return loopCount + 1
}
