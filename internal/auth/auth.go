package auth

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Diegobraun/braunrate/internal/protocol"
	"github.com/Diegobraun/braunrate/internal/runtime"
	"github.com/Diegobraun/braunrate/internal/scenario"
)

type Clock interface {
	Now() time.Time
}

type RunStep func(ctx context.Context, step scenario.Step, values *runtime.Values) (protocol.Response, error)

type Manager struct {
	config  scenario.Auth
	execute RunStep
	clock   Clock

	mu       sync.Mutex
	values   map[string]string
	obtained bool
	obtidoEm time.Time
	Obtains  int64
}

func New(config scenario.Auth, execute RunStep, clock Clock) *Manager {
	return &Manager{config: config, execute: execute, clock: clock, values: map[string]string{}}
}

// Header obtains the token once and refreshes it when it expires, never per
// request: the author declares the refresh and the engine decides when.
func (g *Manager) Header(ctx context.Context, values *runtime.Values) (string, string, error) {
	if g.config.Kind == scenario.AuthBasic {
		user := values.Resolve(g.config.User)
		password := values.Resolve(g.config.Password)
		credential := base64.StdEncoding.EncodeToString([]byte(user + ":" + password))
		return "Authorization", "Basic " + credential, nil
	}

	if err := g.ensureToken(ctx, values); err != nil {
		return "", "", err
	}

	g.mu.Lock()
	obtained := make(map[string]string, len(g.values))
	for name, value := range g.values {
		obtained[name] = value
	}
	g.mu.Unlock()

	values.SetAll(obtained)

	name, model, found := strings.Cut(g.config.Header, ":")
	if !found {
		return "", "", fmt.Errorf("o cabecalho de autenticacao precisa ser \"Nome: valor\", recebido %q", g.config.Header)
	}
	return strings.TrimSpace(name), strings.TrimSpace(values.Resolve(model)), nil
}

func (g *Manager) ensureToken(ctx context.Context, values *runtime.Values) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.obtained && !g.venceu() {
		return nil
	}

	input := values.Values()
	obtainValues := runtime.New(0, 0, input)
	response, err := g.execute(ctx, *g.config.Obtain, obtainValues)
	if err != nil {
		return fmt.Errorf("nao consegui obter a autenticacao (%s): %w", g.config.Obtain.AggregationKey(), err)
	}
	if response.Status >= 400 {
		return fmt.Errorf("a requisicao de autenticacao respondeu %d; confira usuario, senha e caminho em 'autenticacao.obter'", response.Status)
	}

	// Only what the auth request produced is kept. Keeping the whole context
	// would freeze the first iteration's data and reinject it into every other
	// one: the entire load would hit the first CSV row while the report claimed
	// a variety that never happened.
	obtained := map[string]string{}
	for name, value := range obtainValues.Values() {
		if previous, existia := input[name]; existia && previous == value {
			continue
		}
		obtained[name] = value
	}
	g.values = obtained
	g.obtained = true
	g.obtidoEm = g.clock.Now()
	g.Obtains++
	return nil
}

func (g *Manager) venceu() bool {
	if g.config.RefreshAfter <= 0 {
		return false
	}
	return g.clock.Now().Sub(g.obtidoEm) >= g.config.RefreshAfter
}
