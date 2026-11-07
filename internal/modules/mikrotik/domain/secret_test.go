package domain

import (
	"errors"
	"testing"

	"zyad.cloud/internal/modules/mikrotik"
)

func TestPairable(t *testing.T) {
	const isolir = "isolir"
	tests := []struct {
		name    string
		secret  PPPSecret
		wantErr bool
	}{
		{"normal", PPPSecret{Present: true, Profile: "paket-10m"}, false},
		{"needs original profile", PPPSecret{Present: true, Profile: "isolir", NeedsOriginalProfile: true}, true},
		{"not present", PPPSecret{Present: false, Profile: "paket-10m"}, true},
		{"isolir profile with original", PPPSecret{Present: true, Profile: "isolir", OriginalProfile: "paket-10m"}, false},
		{"isolir profile without original", PPPSecret{Present: true, Profile: "isolir"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.secret.Pairable(isolir)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Pairable() err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && !errors.Is(err, mikrotik.ErrSecretNotPairable) {
				t.Fatalf("err = %v, want ErrSecretNotPairable", err)
			}
		})
	}
}
