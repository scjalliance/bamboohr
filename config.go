package bamboohr

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	defaultMaxRetries = 3
	defaultTimeout    = 30 * time.Second
)

// Config configures a Client. APIKey and Subdomain are required; the rest default.
type Config struct {
	APIKey     string       // BambooHR API key (sent as Basic base64(APIKey + ":x"))
	Subdomain  string       // company alias, e.g. "acme" in acme.bamboohr.com
	BaseURL    string       // default https://{Subdomain}.bamboohr.com
	UserAgent  string       // optional
	HTTPClient *http.Client // default: &http.Client{Timeout: defaultTimeout}
	MaxRetries *int         // default 3; pointer so 0 is distinguishable from unset
}

func (c Config) validate() error {
	if c.APIKey == "" {
		return fmt.Errorf("bamboohr: APIKey is required")
	}
	if c.Subdomain == "" {
		return fmt.Errorf("bamboohr: Subdomain is required")
	}
	return nil
}

func (c *Config) applyDefaults() {
	if c.BaseURL == "" {
		c.BaseURL = "https://" + c.Subdomain + ".bamboohr.com"
	}
	c.BaseURL = strings.TrimRight(c.BaseURL, "/")
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{Timeout: defaultTimeout}
	}
	if c.MaxRetries == nil {
		n := defaultMaxRetries
		c.MaxRetries = &n
	}
}
