package main

import "testing"

func TestConfigFromEnvMissing(t *testing.T) {
	t.Setenv("BAMBOOHR_API_KEY", "")
	t.Setenv("BAMBOOHR_SUBDOMAIN", "")
	if _, err := configFromEnv(); err == nil {
		t.Fatal("configFromEnv with no env = nil error, want error")
	}
}

func TestConfigFromEnvOK(t *testing.T) {
	t.Setenv("BAMBOOHR_API_KEY", "k")
	t.Setenv("BAMBOOHR_SUBDOMAIN", "acme")
	cfg, err := configFromEnv()
	if err != nil {
		t.Fatalf("configFromEnv: %v", err)
	}
	if cfg.APIKey != "k" || cfg.Subdomain != "acme" {
		t.Fatalf("cfg = %+v", cfg)
	}
}
