package wait

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Diegobraun/braunrate/internal/protocol"
	"github.com/Diegobraun/braunrate/internal/protocol/transport"
	"github.com/tidwall/gjson"
)

const defaultInterval = 500 * time.Millisecond

// Condition ends an HTTP wait. Many async systems only expose the effect over
// an API: there is no topic to listen to, and without this the end-to-end
// chain cannot be measured on them. Polling is honest as long as the
// granularity is declared: the measured value is always greater than or equal
// to the real one, by up to one poll interval.
type Condition struct {
	Status       int
	Path         string
	Value        string
	BodyContains string
}

func (condition Condition) empty() bool {
	return condition.Status == 0 && condition.Path == "" && condition.BodyContains == ""
}

func (condition Condition) describe() string {
	switch {
	case condition.Path != "":
		return fmt.Sprintf("%s = %q", condition.Path, condition.Value)
	case condition.BodyContains != "":
		return fmt.Sprintf("the body to contain %q", condition.BodyContains)
	default:
		return fmt.Sprintf("status %d", condition.Status)
	}
}

func (condition Condition) satisfied(status int, body []byte) bool {
	switch {
	case condition.Path != "":
		return gjson.GetBytes(body, strings.TrimPrefix(strings.TrimPrefix(condition.Path, "$."), "$")).String() == condition.Value
	case condition.BodyContains != "":
		return strings.Contains(string(body), condition.BodyContains)
	default:
		return status == condition.Status
	}
}

func (implementation *Protocol) awaitOverHTTP(runContext context.Context, request protocol.Request, config *Config) protocol.Response {
	address, err := transport.BuildURL(request.URLBase, config.Path)
	if err != nil {
		return protocol.Response{Class: protocol.ErrConfig, Detail: err.Error()}
	}

	timeout := config.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	interval := config.Interval
	if interval <= 0 {
		interval = defaultInterval
	}

	limit, cancel := context.WithTimeout(runContext, timeout)
	defer cancel()

	var lastStatus int
	var lastBody []byte
	var lastErr string
	polls := 0

	for {
		status, body, err := implementation.poll(limit, address)
		polls++
		if err != nil {
			lastErr = err.Error()
		} else {
			lastStatus, lastBody, lastErr = status, body, ""
			if config.To.satisfied(status, body) {
				return protocol.Response{
					Status: status,
					Body:   body,
					Bytes:  int64(len(body)),
					Class:  protocol.Success,
				}
			}
		}

		select {
		case <-limit.Done():
			return protocol.Response{
				Status: lastStatus,
				Body:   lastBody,
				Class:  protocol.ErrTimeout,
				Detail: waitDetail(config, address, timeout, interval, polls, lastStatus, lastBody, lastErr),
			}
		case <-time.After(interval):
		}
	}
}

func waitDetail(config *Config, address string, timeout, interval time.Duration,
	polls int, status int, body []byte, err string) string {

	if err != "" {
		return fmt.Sprintf("waited %s for %s at %s and the last attempt failed: %s (%d polls every %s)",
			timeout, config.To.describe(), address, err, polls, interval)
	}
	sample := string(body)
	if len(sample) > 120 {
		sample = sample[:120] + "…"
	}
	return fmt.Sprintf("waited %s for %s at %s and the effect never showed up; last response: status %d, body %q (%d polls every %s)",
		timeout, config.To.describe(), address, status, sample, polls, interval)
}

func (implementation *Protocol) poll(runContext context.Context, address string) (int, []byte, error) {
	request, err := http.NewRequestWithContext(runContext, http.MethodGet, address, nil)
	if err != nil {
		return 0, nil, err
	}
	response, err := implementation.client().Do(request)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = response.Body.Close() }()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return response.StatusCode, nil, err
	}
	return response.StatusCode, body, nil
}

func (implementation *Protocol) client() *http.Client {
	implementation.mu.Lock()
	defer implementation.mu.Unlock()
	if implementation.http == nil {
		implementation.http = transport.NewClient(protocol.Options{TLS: implementation.tls})
	}
	return implementation.http
}

func readCondition(key string, raw map[string]string) (Condition, error) {
	condition := Condition{}
	for name, value := range raw {
		switch name {
		case "status":
			number, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				return condition, fmt.Errorf("invalid status in %s: %q (use a number, for example 200)", key, value)
			}
			condition.Status = number
		case "bodyContains":
			condition.BodyContains = value
		default:
			condition.Path = name
			condition.Value = value
		}
	}
	return condition, nil
}

// PollInterval lets the engine declare in the report that the step was
// measured by polling: without it, the interval step would read as target
// latency.
func (config *Config) PollInterval() time.Duration {
	if config.Source != "http" {
		return 0
	}
	if config.Interval > 0 {
		return config.Interval
	}
	return defaultInterval
}
