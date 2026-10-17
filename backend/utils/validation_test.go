package utils

import (
	"strconv"
	"testing"
)

func TestUint64ToUintRejectsOverflow(t *testing.T) {
	if strconv.IntSize == 32 {
		maxUint := ^uint(0)
		if _, err := Uint64ToUint(uint64(maxUint) + 1); err == nil {
			t.Fatal("expected overflow to be rejected")
		}
	}
}

func TestParseUintParam(t *testing.T) {
	value, err := ParseUintParam("42")
	if err != nil || value != 42 {
		t.Fatalf("ParseUintParam(42) = %d, %v", value, err)
	}
	if _, err := ParseUintParam("-1"); err == nil {
		t.Fatal("expected negative values to be rejected")
	}
	if strconv.IntSize == 32 {
		if _, err := ParseUintParam("4294967296"); err == nil {
			t.Fatal("expected 32-bit overflow to be rejected")
		}
	}
}