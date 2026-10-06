package config

import "testing"

func TestLoadIncludesSelfServeDefaults(t *testing.T) {
	t.Setenv("SELF_SERVE_BOT_EMAIL", "")
	t.Setenv("SELF_SERVE_DEAL_OWNER_EMAIL", "")
	cfg := Load()
	if cfg.SelfServe.BotEmail != "self-serve-bot@zyad.cloud" || cfg.SelfServe.DealOwnerEmail != "" {
		t.Fatalf("SelfServe = %+v", cfg.SelfServe)
	}
	t.Setenv("SELF_SERVE_BOT_EMAIL", "bot@example.com")
	t.Setenv("SELF_SERVE_DEAL_OWNER_EMAIL", "owner@example.com")
	cfg = Load()
	if cfg.SelfServe.BotEmail != "bot@example.com" || cfg.SelfServe.DealOwnerEmail != "owner@example.com" {
		t.Fatalf("SelfServe = %+v", cfg.SelfServe)
	}
}
