package service

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeUsername(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr error
	}{
		{name: "lowercases", raw: "Budi.Santoso", want: "budi.santoso"},
		{name: "trims", raw: "  budi_s  ", want: "budi_s"},
		{name: "min length 3", raw: "abc", want: "abc"},
		{name: "too short", raw: "ab", wantErr: ErrUsernameInvalid},
		{name: "30 chars ok", raw: strings.Repeat("a", 30), want: strings.Repeat("a", 30)},
		{name: "31 chars", raw: strings.Repeat("a", 31), wantErr: ErrUsernameInvalid},
		{name: "double dot", raw: "budi..s", wantErr: ErrUsernameInvalid},
		{name: "email shaped", raw: "budi@x.com", wantErr: ErrUsernameInvalid},
		{name: "digits only", raw: "12345", wantErr: ErrUsernameInvalid},
		{name: "leading underscore", raw: "_budi", wantErr: ErrUsernameInvalid},
		{name: "trailing dot", raw: "budi.", wantErr: ErrUsernameInvalid},
		{name: "builtin reserved", raw: "admin", wantErr: ErrUsernameReserved},
		{name: "builtin reserved mixed case", raw: "Admin", wantErr: ErrUsernameReserved},
		{name: "configured reserved", raw: "www", wantErr: ErrUsernameReserved},
		{name: "configured reserved is normalized", raw: "tenantx", wantErr: ErrUsernameReserved},
	}
	reserved := []string{"www", " TenantX "}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeUsername(tt.raw, reserved)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}
