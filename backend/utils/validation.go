package utils

import (
	"fmt"
	"strconv"
)

// Uint64ToUint converts a uint64 to uint without allowing truncation on
// platforms where uint is narrower.
func Uint64ToUint(value uint64) (uint, error) {
	maxUint := ^uint(0)
	if value > uint64(maxUint) {
		return 0, fmt.Errorf("value %d overflows uint", value)
	}
	return uint(value), nil
}

// ParseUintParam parses a decimal identifier and safely converts it to uint.
func ParseUintParam(value string) (uint, error) {
	parsed, err := strconv.ParseUint(value, 10, strconv.IntSize)
	if err != nil {
		return 0, err
	}
	return Uint64ToUint(parsed)
}