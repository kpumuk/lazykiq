// Package mathutil provides common mathematical utility functions.
package mathutil

import "cmp"

// Clamp restricts a value to be within a specified range.
// Returns low if val < low, high if val > high, otherwise returns val.
func Clamp[T cmp.Ordered](val, low, high T) T {
	if val < low {
		return low
	}
	if val > high {
		return high
	}
	return val
}
