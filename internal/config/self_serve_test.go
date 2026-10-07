package config

import "testing"

func TestLoadIncludesSelfServeDefaults(t *testing.T) {
	t.Setenv("SELF_SERVE_BOT_EMAIL", "")
	t.Setenv("SELF_SERVE_DEAL_OWNER_EMAIL", "")
	cfg := Load()
	if cfg.SelfServe.BotEmail != "self-serve-bot@zyad.cloud" || cfg.SelfServe.DealOwnerEmail != "" {
		t.Fatalf("SelfServe = %+v", cfg.SelfServe)
	}
	if cfg.SelfServe.FreeProductSKU != "FREE" {
		t.Fatalf("FreeProductSKU = %q, want FREE", cfg.SelfServe.FreeProductSKU)
	}
	t.Setenv("SELF_SERVE_FREE_PRODUCT_SKU", "STARTER")
	if got := Load().SelfServe.FreeProductSKU; got != "STARTER" {
		t.Fatalf("FreeProductSKU = %q, want STARTER", got)
	}
	t.Setenv("SELF_SERVE_BOT_EMAIL", "bot@example.com")
	t.Setenv("SELF_SERVE_DEAL_OWNER_EMAIL", "owner@example.com")
	cfg = Load()
	if cfg.SelfServe.BotEmail != "bot@example.com" || cfg.SelfServe.DealOwnerEmail != "owner@example.com" {
		t.Fatalf("SelfServe = %+v", cfg.SelfServe)
	}
}

func TestLoadSelfServeGraceDays(t *testing.T) {
	cases := []struct {
		env     string
		want    int
		invalid bool
	}{
		{"", 7, false}, {"0", 0, false}, {"14", 14, false}, {"60", 60, false},
		{"61", 7, true}, {"-1", 7, true}, {"abc", 7, true},
	}
	for _, c := range cases {
		t.Setenv("SELF_SERVE_GRACE_DAYS", c.env)
		got := Load().SelfServe
		if got.GraceDays != c.want || got.GraceDaysInvalid != c.invalid {
			t.Errorf("SELF_SERVE_GRACE_DAYS=%q → %d invalid=%v, want %d invalid=%v", c.env, got.GraceDays, got.GraceDaysInvalid, c.want, c.invalid)
		}
	}
}
