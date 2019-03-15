// Package strutil implements a collection of utility functions for
// manipulating strings and lists of strings.
package strutil

// Contains returns true if the provided string is in the provided string
// slice.
func Contains(ys []string, x string) bool {
	for _, y := range ys {
		if x == y {
			return true
		}
	}
	return false
}

// Dedup returns a new slice with any duplicates removed.
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

// Default returns a fallback value when the provided value is equal to any
// of the provided zero values.
func Default(val, fallback string, zeroValues ...string) string {
	for _, zeroValue := range zeroValues {
		if val == zeroValue {
			return fallback
		}
	}

	return val
}
