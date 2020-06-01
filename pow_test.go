package nanopow

import (
	"testing"
)

func TestCalculateDifficulty(t *testing.T) {
	if CalculateDifficulty(0) != 18446743798831644672 {
		t.Error("invalid calculation")
	}
}