package bamboohr

import "testing"

func TestConfigValidate(t *testing.T) {
	cases := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{"ok", Config{APIKey: "k", Subdomain: "acme"}, false},
		{"missing api key", Config{Subdomain: "acme"}, true},
		{"missing subdomain", Config{APIKey: "k"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.cfg.validate(); (err != nil) != tc.wantErr {
				t.Fatalf("validate() err=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

func TestConfigApplyDefaults(t *testing.T) {
	c := Config{APIKey: "k", Subdomain: "acme"}
	c.applyDefaults()
	if c.BaseURL != "https://acme.bamboohr.com" {
		t.Fatalf("BaseURL = %q, want https://acme.bamboohr.com", c.BaseURL)
	}
	if c.HTTPClient == nil {
		t.Fatal("HTTPClient not defaulted")
	}
	if c.MaxRetries == nil || *c.MaxRetries != defaultMaxRetries {
		t.Fatalf("MaxRetries not defaulted to %d", defaultMaxRetries)
	}
}

func TestConfigApplyDefaultsKeepsOverrides(t *testing.T) {
	zero := 0
	c := Config{APIKey: "k", Subdomain: "acme", BaseURL: "https://x/", MaxRetries: &zero}
	c.applyDefaults()
	if c.BaseURL != "https://x" { // trailing slash trimmed
		t.Fatalf("BaseURL = %q, want trimmed override", c.BaseURL)
	}
	if *c.MaxRetries != 0 {
		t.Fatalf("MaxRetries override lost")
	}
}
