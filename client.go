package bamboohr

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Client talks to the BambooHR API.
type Client struct {
	cfg       Config
	baseDelay time.Duration // base backoff between retries (overridable in tests)
}

// New validates the config, applies defaults, and returns a Client.
func New(c Config) (*Client, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	c.applyDefaults()
	return &Client{cfg: c, baseDelay: 500 * time.Millisecond}, nil
}

func (c *Client) authHeader() string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(c.cfg.APIKey+":x"))
}

func (c *Client) urlFor(path string) string {
	return c.cfg.BaseURL + "/" + path
}

// get is a convenience for GET with query params.
func (c *Client) get(ctx context.Context, path string, query url.Values, out any) error {
	if len(query) > 0 {
		path += "?" + query.Encode()
	}
	return c.do(ctx, http.MethodGet, path, nil, out)
}

// do performs an authenticated JSON request with retry/backoff on 429/503 and
// decodes a 2xx JSON body into out (if non-nil). Non-2xx → *APIError.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	return c.doWith(ctx, method, path, body, out, false)
}

// ErrWriteOutcomeUnknown wraps a failed non-idempotent write (a table-row add)
// that may or may not have been applied: a network error after the request
// was sent, or a 503. Re-read before trying again; retrying blindly can
// duplicate the row. Other errors mean the write was not applied.
var ErrWriteOutcomeUnknown = errors.New("bamboohr: write outcome unknown")

// notSent reports a transport error that happened before the request reached
// the server (dial or DNS failure), so retrying a write is safe.
func notSent(err error) bool {
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return true
	}
	var op *net.OpError
	return errors.As(err, &op) && op.Op == "dial"
}

// doWith is do with a retry policy. A non-idempotent request (nonIdempotent)
// is retried only on 429, where BambooHR states it rejected the request, and
// on dial or DNS failures, where it never reached the server. Any other
// network error, or a 503, may come after the server applied it, so a retry
// could repeat the write (a duplicate table row); those return at once,
// wrapping ErrWriteOutcomeUnknown.
func (c *Client) doWith(ctx context.Context, method, path string, body, out any, nonIdempotent bool) error {
	var bodyBytes []byte
	if body != nil {
		var err error
		if bodyBytes, err = json.Marshal(body); err != nil {
			return fmt.Errorf("bamboohr: marshal body: %w", err)
		}
	}

	maxAttempts := *c.cfg.MaxRetries + 1
	var lastErr error
	var delayHint time.Duration // Retry-After from the previous attempt, if any
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			if err := c.wait(ctx, attempt, delayHint); err != nil {
				return err
			}
		}

		req, err := http.NewRequestWithContext(ctx, method, c.urlFor(path), bytesReader(bodyBytes))
		if err != nil {
			return fmt.Errorf("bamboohr: new request: %w", err)
		}
		req.Header.Set("Authorization", c.authHeader())
		req.Header.Set("Accept", "application/json")
		if bodyBytes != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if c.cfg.UserAgent != "" {
			req.Header.Set("User-Agent", c.cfg.UserAgent)
		}

		resp, err := c.cfg.HTTPClient.Do(req)
		if err != nil {
			// Transient network failures (reset, timeout, DNS blip) are retryable;
			// a cancelled context is caught by wait() at the top of the next attempt.
			lastErr = fmt.Errorf("bamboohr: %s %s: %w", method, path, err)
			if nonIdempotent && !notSent(err) {
				return fmt.Errorf("%w: %w", ErrWriteOutcomeUnknown, lastErr)
			}
			delayHint = 0
			continue
		}

		// 429 (too fast) is always retryable. 503 (gateway overwhelmed) is
		// retryable only for idempotent requests.
		if resp.StatusCode == http.StatusTooManyRequests ||
			(resp.StatusCode == http.StatusServiceUnavailable && !nonIdempotent) {
			delayHint = parseRetryAfter(resp.Header.Get("Retry-After"))
			lastErr = &APIError{Status: resp.StatusCode, Message: http.StatusText(resp.StatusCode),
				BambooHRMessage: resp.Header.Get("X-BambooHR-Error-Message")}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			continue
		}

		if resp.StatusCode == http.StatusServiceUnavailable && nonIdempotent {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			return fmt.Errorf("%w: %w", ErrWriteOutcomeUnknown, &APIError{Status: resp.StatusCode,
				Message: http.StatusText(resp.StatusCode), BambooHRMessage: resp.Header.Get("X-BambooHR-Error-Message")})
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			resp.Body.Close()
			return &APIError{Status: resp.StatusCode, Message: http.StatusText(resp.StatusCode),
				BambooHRMessage: resp.Header.Get("X-BambooHR-Error-Message")}
		}
		if out == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			return nil
		}
		err = json.NewDecoder(resp.Body).Decode(out)
		resp.Body.Close()
		if err != nil {
			return fmt.Errorf("bamboohr: decode response: %w", err)
		}
		return nil
	}
	return lastErr // retries exhausted → the last rate-limit *APIError
}

// wait sleeps before a retry: the Retry-After hint if present, else capped
// exponential backoff (baseDelay × 2^(attempt-1)). Cancellable via ctx.
func (c *Client) wait(ctx context.Context, attempt int, hint time.Duration) error {
	d := hint
	if d <= 0 {
		d = c.baseDelay * time.Duration(1<<uint(attempt-1)) // 1x, 2x, 4x, ...
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func bytesReader(b []byte) io.Reader {
	if b == nil {
		return nil
	}
	return bytes.NewReader(b)
}

func parseRetryAfter(h string) time.Duration {
	// Handles integer-seconds Retry-After only; HTTP-date values fall back to exponential backoff (return 0).
	if h == "" {
		return 0
	}
	if secs, err := strconv.Atoi(h); err == nil {
		return time.Duration(secs) * time.Second
	}
	return 0
}
