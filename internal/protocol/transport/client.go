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
func NewClient(options protocol.Options) *http.Client {
	transporte := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConns:          0,
		MaxIdleConnsPerHost:   65536,
		MaxConnsPerHost:       options.ConnsPerHost,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
		ForceAttemptHTTP2:     true,
		TLSClientConfig:       options.TLS,
	}
	client := &http.Client{Transport: transporte, Timeout: options.Timeout}
	if options.KeepCookies {
		if jar, err := cookiejar.New(nil); err == nil {
			client.Jar = jar
		}
	}
	if !options.FollowRedirects {
		client.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
		return client
	}
	maximum := options.MaxRedirects
	client.CheckRedirect = func(_ *http.Request, anteriores []*http.Request) error {
		if len(anteriores) >= maximum {
			return fmt.Errorf("more than %d redirects", maximum)
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
		return "", fmt.Errorf("step with the relative path %q and a scenario with no target", path)
	}
	enderecoBase, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("invalid target: %q", base)
	}
	relative, err := url.Parse(path)
	if err != nil {
		return "", fmt.Errorf("invalid path: %q", path)
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
	if summary := summarizeTLS(text); summary != "" {
		return summary
	}
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
	// Um cookie de sessao e credencial como o Bearer e, desde que o gravador
	// passou a correlacionar sessao, e o que aparece no depurar de qualquer
	// jornada web. O nome do par fica: e ele que diz o que esta sendo mandado.
	if lowerName == "cookie" || lowerName == "set-cookie" {
		return maskCookiePairs(value)
	}
	if lowerName != "authorization" && !strings.Contains(lowerName, "token") &&
		!strings.Contains(lowerName, "senha") && !strings.Contains(lowerName, "secret") &&
		!strings.Contains(lowerName, "api-key") && !strings.Contains(lowerName, "apikey") {
		return value
	}
	prefix, rest, found := strings.Cut(value, " ")
	if !found {
		prefix, rest = "", value
	}
	if len(rest) <= 6 {
		return strings.TrimSpace(prefix + " ***")
	}
	return strings.TrimSpace(prefix + " " + rest[:6] + "… (" + fmt.Sprint(len(rest)) + " characters)")
}

func maskCookiePairs(value string) string {
	pairs := strings.Split(value, ";")
	for index, pair := range pairs {
		name, content, found := strings.Cut(strings.TrimSpace(pair), "=")
		if !found || content == "" {
			pairs[index] = strings.TrimSpace(pair)
			continue
		}
		if len(content) <= 6 {
			pairs[index] = name + "=***"
			continue
		}
		pairs[index] = name + "=" + content[:6] + "… (" + fmt.Sprint(len(content)) + " characters)"
	}
	return strings.Join(pairs, "; ")
}

// The raw x509 error is long, ends in the part that matters, and got cut by the
// column that shows it — the reader was left with the URL and no diagnosis. It
// also says nothing about the way out, and until there was a 'tls' block there
// was none; now there is one and the message names it.
func summarizeTLS(text string) string {
	switch {
	case strings.Contains(text, "certificate signed by unknown authority"):
		return "certificate signed by a CA this machine does not know — declare tls: { ca: /path/to/ca.pem }"
	case strings.Contains(text, "cannot validate certificate for"):
		return "the target certificate is not valid for the address called — use the name that is in the certificate"
	case strings.Contains(text, "certificate has expired"):
		return "the target certificate is expired or not valid yet"
	case strings.Contains(text, "tls: bad certificate"), strings.Contains(text, "certificate required"):
		return "the target asked for a client certificate — declare tls: { certificate: /path/client.pem, key: /path/client.key }"
	}
	return ""
}
