package api

import (
	"slices"
	"strings"
	"testing"
)

func TestValidateBulkClose(t *testing.T) {
	in := bulkCloseInput{IDs: []int64{3, 1, 3, 0, -2, 1, 7}, Reason: "  done  "}
	if err := validateBulkClose(&in); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(in.IDs, []int64{3, 1, 7}) || in.Reason != "done" {
		t.Fatalf("got ids %v, reason %q", in.IDs, in.Reason)
	}

	tooMany := make([]int64, maxBulkClose+1)
	for i := range tooMany {
		tooMany[i] = int64(i + 1)
	}
	for name, bad := range map[string]bulkCloseInput{
		"no ids":      {},
		"only zeroes": {IDs: []int64{0}},
		"too many":    {IDs: tooMany},
		"long reason": {IDs: []int64{1}, Reason: strings.Repeat("a", 501)},
	} {
		if err := validateBulkClose(&bad); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}
