package copilotruntime

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRuntimeAvailability(t *testing.T) {
	require.ErrorContains(
		t,
		Run(t.Context(), RuntimeRequest{}),
		"requires application tools",
	)
}
