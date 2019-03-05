package strutil

func Contains(ys []string, x string) bool {
	for _, y := range ys {
		if x == y {
			return true
		}
	}
	return false
}

func Dedup(xs []string) []string {
	xsSet := make(map[string]struct{}, 0)
	for _, x := range xs {
		xsSet[x] = struct{}{}
	}

	ys := make([]string, 0, len(xsSet))
	for x := range xsSet {
		ys = append(ys, x)
	}

	return ys
}
