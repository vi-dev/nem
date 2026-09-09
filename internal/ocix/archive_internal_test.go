package ocix

import (
	"errors"
	"fmt"
	"testing"

	"oras.land/oras-go/v2/errdef"
	"oras.land/oras-go/v2/registry/remote/errcode"
)

func TestArchiveAbsent(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"errdef not found", errdef.ErrNotFound, true},
		{"403 forbidden", &errcode.ErrorResponse{StatusCode: 403}, true},
		{"404 not found", &errcode.ErrorResponse{StatusCode: 404}, true},
		{"401 unauthorized", &errcode.ErrorResponse{StatusCode: 401}, false},
		{"plain error", errors.New("x"), false},
		{"wrapped 403", fmt.Errorf("ctx: %w", &errcode.ErrorResponse{StatusCode: 403}), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := archiveAbsent(c.err); got != c.want {
				t.Fatalf("archiveAbsent(%v) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}
