package sockparty_test

import (
	"testing"

	"github.com/izzymg/sockparty"
)

func TestDefaultOptions(t *testing.T) {
	opts := sockparty.DefaultOptions()
	if opts.AllowCrossOrigin {
		t.Fatal("Cross origin should be disabled by default")
	}
}
