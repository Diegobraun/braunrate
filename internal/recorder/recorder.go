// Package recorder listens as an HTTP proxy and turns what passed through it
// into a scenario. It records what the client actually sent, and everything it
// could not record it says out loud — a recorder that drops traffic in silence
// produces a scenario that measures less than the person thinks.
package recorder

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

type Entry struct {
	Method          string
	URL             *url.URL
	Headers         map[string]string
	Body            string
	Status          int
	ResponseBody    []byte
	ResponseCookies []string
	ContentType     string
}

type Options struct {
	Address string
	Hosts   []string
	Ignore  []string
}

type Recorder struct {
	options   Options
	transport *http.Transport

	mu       sync.Mutex
	entries  []Entry
	dropped  map[string]int
	tunneled map[string]int
	allowed  map[string]bool
}

func New(options Options) *Recorder {
	allowed := map[string]bool{}
	for _, host := range options.Hosts {
		if trimmed := strings.TrimSpace(host); trimmed != "" {
			allowed[strings.ToLower(trimmed)] = true
		}
	}
	return &Recorder{
		options:   options,
		transport: &http.Transport{Proxy: nil, DisableCompression: true},
		dropped:   map[string]int{},
		tunneled:  map[string]int{},
		allowed:   allowed,
	}
}

func (recorder *Recorder) Entries() []Entry {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	return append([]Entry{}, recorder.entries...)
}

func (recorder *Recorder) Dropped() map[string]int {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	clone := map[string]int{}
	for reason, count := range recorder.dropped {
		clone[reason] = count
	}
	return clone
}

func (recorder *Recorder) Tunneled() map[string]int {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	clone := map[string]int{}
	for host, count := range recorder.tunneled {
		clone[host] = count
	}
	return clone
}

// Serve blocks until the context is cancelled, which is what Ctrl+C does.
func (recorder *Recorder) Serve(runContext context.Context, ready func(address string)) error {
	listener, err := net.Listen("tcp", recorder.options.Address)
	if err != nil {
		return fmt.Errorf("could not listen on %s: %w", recorder.options.Address, err)
	}
	if ready != nil {
		ready(listener.Addr().String())
	}

	server := &http.Server{Handler: http.HandlerFunc(recorder.handle), ReadHeaderTimeout: 30 * time.Second}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()

	select {
	case <-runContext.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
		return nil
	case err := <-done:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

func (recorder *Recorder) handle(writer http.ResponseWriter, request *http.Request) {
	if request.Method == http.MethodConnect {
		recorder.tunnel(writer, request)
		return
	}
	if request.URL.Host == "" {
		http.Error(writer, "braunrate is listening as a proxy; set the client to use this address as its HTTP proxy", http.StatusBadRequest)
		return
	}

	var body []byte
	if request.Body != nil {
		body, _ = io.ReadAll(io.LimitReader(request.Body, maxBody))
		_ = request.Body.Close()
	}

	outgoing, err := http.NewRequestWithContext(request.Context(), request.Method, request.URL.String(), strings.NewReader(string(body)))
	if err != nil {
		http.Error(writer, err.Error(), http.StatusBadGateway)
		return
	}
	for name, values := range request.Header {
		if hopByHop[strings.ToLower(name)] {
			continue
		}
		for _, value := range values {
			outgoing.Header.Add(name, value)
		}
	}

	response, err := recorder.transport.RoundTrip(outgoing)
	if err != nil {
		http.Error(writer, fmt.Sprintf("I could not talk to %s: %v", request.URL.Host, err), http.StatusBadGateway)
		recorder.drop("the target did not answer")
		return
	}
	defer func() { _ = response.Body.Close() }()

	answer, _ := io.ReadAll(io.LimitReader(response.Body, maxBody))
	recorder.record(request, body, response, answer)

	for name, values := range response.Header {
		if hopByHop[strings.ToLower(name)] {
			continue
		}
		for _, value := range values {
			writer.Header().Add(name, value)
		}
	}
	writer.WriteHeader(response.StatusCode)
	_, _ = writer.Write(answer)
}

// The tunnel keeps the client working, and the count keeps the omission
// visible: recording inside TLS needs a certificate authority of our own, which
// is declared out of scope.
func (recorder *Recorder) tunnel(writer http.ResponseWriter, request *http.Request) {
	host := request.URL.Host
	if host == "" {
		host = request.Host
	}
	recorder.mu.Lock()
	recorder.tunneled[hostOnly(host)]++
	recorder.mu.Unlock()

	upstream, err := net.DialTimeout("tcp", host, 10*time.Second)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusBadGateway)
		return
	}
	hijacker, canHijack := writer.(http.Hijacker)
	if !canHijack {
		_ = upstream.Close()
		http.Error(writer, "the connection cannot be hijacked", http.StatusInternalServerError)
		return
	}
	client, _, err := hijacker.Hijack()
	if err != nil {
		_ = upstream.Close()
		return
	}
	_, _ = client.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))

	go func() {
		defer func() { _ = upstream.Close() }()
		defer func() { _ = client.Close() }()
		_, _ = io.Copy(upstream, client)
	}()
	go func() {
		defer func() { _ = upstream.Close() }()
		defer func() { _ = client.Close() }()
		_, _ = io.Copy(client, upstream)
	}()
}

const maxBody = 1 << 20

var hopByHop = map[string]bool{
	"connection": true, "proxy-connection": true, "keep-alive": true,
	"transfer-encoding": true, "te": true, "trailer": true, "upgrade": true,
	"proxy-authenticate": true, "proxy-authorization": true,
	"accept-encoding": true, "content-length": true,
}

func (recorder *Recorder) record(request *http.Request, body []byte, response *http.Response, answer []byte) {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()

	if len(recorder.allowed) == 0 {
		recorder.allowed[hostOnly(request.URL.Host)] = true
	}
	if reason, noise := recorder.classify(request); noise {
		recorder.dropped[reason]++
		return
	}

	headers := map[string]string{}
	for name := range request.Header {
		if hopByHop[strings.ToLower(name)] || ignoredHeaders[strings.ToLower(name)] {
			continue
		}
		headers[name] = request.Header.Get(name)
	}

	recorder.entries = append(recorder.entries, Entry{
		Method:          request.Method,
		URL:             cloneURL(request.URL),
		Headers:         headers,
		Body:            string(body),
		Status:          response.StatusCode,
		ResponseBody:    answer,
		ResponseCookies: response.Header.Values("Set-Cookie"),
		ContentType:     response.Header.Get("Content-Type"),
	})
}

func (recorder *Recorder) drop(reason string) {
	recorder.mu.Lock()
	recorder.dropped[reason]++
	recorder.mu.Unlock()
}

// Recorded noise costs more than a missing step: it turns into load against a
// CDN, and the report then describes something nobody wanted to measure.
func (recorder *Recorder) classify(request *http.Request) (string, bool) {
	host := hostOnly(request.URL.Host)
	if !recorder.allowed[host] {
		return fmt.Sprintf("an outside domain (%s)", host), true
	}
	if request.Method == http.MethodOptions {
		return "a CORS preflight", true
	}
	path := strings.ToLower(request.URL.Path)
	if isTelemetry(host, path) {
		return "telemetry", true
	}
	if isStatic(path) {
		return "a static asset", true
	}
	for _, pattern := range recorder.options.Ignore {
		pattern = strings.TrimSpace(pattern)
		if pattern != "" && strings.Contains(request.URL.Path, pattern) {
			return fmt.Sprintf("asked for by -ignore (%s)", pattern), true
		}
	}
	return "", false
}

var staticExtensions = []string{
	".js", ".mjs", ".css", ".map", ".ico", ".png", ".jpg", ".jpeg", ".gif",
	".svg", ".webp", ".avif", ".bmp", ".woff", ".woff2", ".ttf", ".otf", ".eot",
	".mp4", ".webm", ".mp3", ".wav",
}

func isStatic(path string) bool {
	if strings.HasSuffix(path, "/favicon.ico") || path == "/robots.txt" {
		return true
	}
	for _, extension := range staticExtensions {
		if strings.HasSuffix(path, extension) {
			return true
		}
	}
	return false
}

var telemetryHosts = []string{
	"google-analytics.com", "googletagmanager.com", "doubleclick.net",
	"segment.io", "segment.com", "sentry.io", "datadoghq.com", "newrelic.com",
	"nr-data.net", "hotjar.com", "mixpanel.com", "amplitude.com",
	"facebook.com", "facebook.net", "clarity.ms", "fullstory.com",
}

var telemetryPaths = []string{"/analytics", "/telemetry", "/collect", "/beacon", "/rum", "/gtm.js"}

func isTelemetry(host, path string) bool {
	for _, known := range telemetryHosts {
		if host == known || strings.HasSuffix(host, "."+known) {
			return true
		}
	}
	for _, known := range telemetryPaths {
		if strings.HasPrefix(path, known) {
			return true
		}
	}
	return false
}

var ignoredHeaders = map[string]bool{
	"host": true, "content-length": true, "accept-encoding": true,
	"sec-fetch-dest": true, "sec-fetch-mode": true, "sec-fetch-site": true,
	"sec-fetch-user": true, "sec-ch-ua": true, "sec-ch-ua-mobile": true,
	"sec-ch-ua-platform": true, "upgrade-insecure-requests": true,
	"user-agent": true, "accept-language": true, "referer": true, "origin": true,
	"dnt": true, "priority": true, "pragma": true, "cache-control": true,
}

func hostOnly(hostPort string) string {
	if host, _, err := net.SplitHostPort(hostPort); err == nil {
		return strings.ToLower(host)
	}
	return strings.ToLower(hostPort)
}

func cloneURL(original *url.URL) *url.URL {
	clone := *original
	return &clone
}

// DroppedLines turns the counters into what goes on screen, sorted so two runs
// of the same session print the same thing.
func DroppedLines(dropped map[string]int) []string {
	reasons := make([]string, 0, len(dropped))
	for reason := range dropped {
		reasons = append(reasons, reason)
	}
	sort.Slice(reasons, func(first, second int) bool {
		if dropped[reasons[first]] != dropped[reasons[second]] {
			return dropped[reasons[first]] > dropped[reasons[second]]
		}
		return reasons[first] < reasons[second]
	})
	lines := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		lines = append(lines, fmt.Sprintf("%d %s", dropped[reason], reason))
	}
	return lines
}
