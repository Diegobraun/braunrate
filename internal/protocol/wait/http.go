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

// Boa parte dos sistemas assincronos so mostra o efeito por API: nao ha topico
// para escutar, e sem isto a cadeia ponta a ponta nao se mede neles. A medicao
// por sondagem e honesta desde que a granularidade seja declarada — o valor
// medido e sempre maior ou igual ao real, ate um intervalo de sondagem.
type Condition struct {
	Status       int
	Path         string
	Value        string
	BodyContains string
}

func (c Condition) empty() bool {
	return c.Status == 0 && c.Path == "" && c.BodyContains == ""
}

func (c Condition) describe() string {
	switch {
	case c.Path != "":
		return fmt.Sprintf("%s = %q", c.Path, c.Value)
	case c.BodyContains != "":
		return fmt.Sprintf("o corpo conter %q", c.BodyContains)
	default:
		return fmt.Sprintf("status %d", c.Status)
	}
}

func (c Condition) satisfied(status int, body []byte) bool {
	switch {
	case c.Path != "":
		return gjson.GetBytes(body, strings.TrimPrefix(strings.TrimPrefix(c.Path, "$."), "$")).String() == c.Value
	case c.BodyContains != "":
		return strings.Contains(string(body), c.BodyContains)
	default:
		return status == c.Status
	}
}

func (p *Protocol) awaitOverHTTP(ctx context.Context, request protocol.Request, config *Config) protocol.Response {
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

	limit, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var lastStatus int
	var lastBody []byte
	var lastErr string
	polls := 0

	for {
		status, body, err := p.poll(limit, address)
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
		return fmt.Sprintf("esperei %s por %s em %s e a ultima tentativa falhou: %s (%d sondagens a cada %s)",
			timeout, config.To.describe(), address, err, polls, interval)
	}
	sample := string(body)
	if len(sample) > 120 {
		sample = sample[:120] + "…"
	}
	return fmt.Sprintf("esperei %s por %s em %s e o efeito nao apareceu; ultima resposta: status %d, corpo %q (%d sondagens a cada %s)",
		timeout, config.To.describe(), address, status, sample, polls, interval)
}

func (p *Protocol) poll(ctx context.Context, address string) (int, []byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return 0, nil, err
	}
	response, err := p.client().Do(request)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return response.StatusCode, nil, err
	}
	return response.StatusCode, body, nil
}

func (p *Protocol) client() *http.Client {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.http == nil {
		p.http = transport.NewClient(protocol.Options{})
	}
	return p.http
}

func readCondition(key string, raw map[string]string) (Condition, error) {
	condition := Condition{}
	for name, value := range raw {
		switch name {
		case "status":
			number, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				return condition, fmt.Errorf("status invalido em %s: %q (use um numero, por exemplo 200)", key, value)
			}
			condition.Status = number
		case "corpo_contem":
			condition.BodyContains = value
		default:
			condition.Path = name
			condition.Value = value
		}
	}
	return condition, nil
}

// O motor usa isto para declarar no relatorio que aquele passo foi medido por
// sondagem — sem a declaracao, o degrau do intervalo viraria latencia do alvo.
func (c *Config) PollInterval() time.Duration {
	if c.Source != "http" {
		return 0
	}
	if c.Interval > 0 {
		return c.Interval
	}
	return defaultInterval
}
