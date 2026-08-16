package transport

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"github.com/Diegobraun/braunrate/internal/protocol"
)

// NewClient is the single place a client is built: HTTP and GraphQL need the
// same connection pool, and different pools per protocol would produce
// different numbers for the same load with nothing in the scenario explaining
// the difference.
func NewClient(opts protocol.Options) *http.Client {
	transporte := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConns:          0,
		MaxIdleConnsPerHost:   65536,
		MaxConnsPerHost:       opts.ConnsPerHost,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
		ForceAttemptHTTP2:     true,
	}
	client := &http.Client{Transport: transporte, Timeout: opts.Timeout}
	if opts.KeepCookies {
		if jar, err := cookiejar.New(nil); err == nil {
			client.Jar = jar
		}
	}
	if !opts.FollowRedirects {
		client.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
		return client
	}
	maximum := opts.MaxRedirects
	client.CheckRedirect = func(_ *http.Request, anteriores []*http.Request) error {
		if len(anteriores) >= maximum {
			return fmt.Errorf("mais de %d redirects", maximum)
		}
		return nil
	}
	return client
}

func BuildURL(base, path string) (string, error) {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path, nil
	}
	if base == "" {
		return "", fmt.Errorf("passo com caminho relativo %q e cenario sem alvo", path)
	}
	enderecoBase, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("alvo invalido: %q", base)
	}
	relative, err := url.Parse(path)
	if err != nil {
		return "", fmt.Errorf("caminho invalido: %q", path)
	}
	return enderecoBase.ResolveReference(relative).String(), nil
}

func Classify(err error) protocol.ErrorClass {
	if errors.Is(err, context.DeadlineExceeded) {
		return protocol.ErrTimeout
	}
	var networkErr net.Error
	if errors.As(err, &networkErr) && networkErr.Timeout() {
		return protocol.ErrTimeout
	}
	return protocol.ErrNetwork
}

func SummarizeError(err error) string {
	text := err.Error()
	for _, known := range []string{"connection refused", "connection reset", "no such host",
		"too many open files", "cannot assign requested address", "context deadline exceeded",
		"EOF", "broken pipe"} {
		if strings.Contains(text, known) {
			return known
		}
	}
	if len(text) > 120 {
		return text[:120]
	}
	return text
}

// MaskSecret trims tokens and passwords: debug output tends to end up in
// tickets and screenshots.
func MaskSecret(name, value string) string {
	lowerName := strings.ToLower(name)
	if lowerName != "authorization" && !strings.Contains(lowerName, "token") &&
		!strings.Contains(lowerName, "senha") && !strings.Contains(lowerName, "secret") {
		return value
	}
	prefix, rest, found := strings.Cut(value, " ")
	if !found {
		prefix, rest = "", value
	}
	if len(rest) <= 6 {
		return strings.TrimSpace(prefix + " ***")
	}
	return strings.TrimSpace(prefix + " " + rest[:6] + "… (" + fmt.Sprint(len(rest)) + " caracteres)")
}
