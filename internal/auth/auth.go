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

type RunStep func(runContext context.Context, step scenario.Step, values *runtime.Values) (protocol.Response, error)

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
func (manager *Manager) Header(runContext context.Context, values *runtime.Values) (string, string, error) {
	if manager.config.Kind == scenario.AuthBasic {
		user := values.Resolve(manager.config.User)
		password := values.Resolve(manager.config.Password)
		credential := base64.StdEncoding.EncodeToString([]byte(user + ":" + password))
		return "Authorization", "Basic " + credential, nil
	}

	if err := manager.ensureToken(runContext, values); err != nil {
		return "", "", err
	}

	manager.mu.Lock()
	obtained := make(map[string]string, len(manager.values))
	for name, value := range manager.values {
		obtained[name] = value
	}
	manager.mu.Unlock()

	values.SetAll(obtained)

	name, model, found := strings.Cut(manager.config.Header, ":")
	if !found {
		return "", "", fmt.Errorf("o cabeçalho de autenticação precisa ser \"Nome: valor\", recebido %q", manager.config.Header)
	}
	return strings.TrimSpace(name), strings.TrimSpace(values.Resolve(model)), nil
}

func (manager *Manager) ensureToken(runContext context.Context, values *runtime.Values) error {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	if manager.obtained && !manager.venceu() {
		return nil
	}

	input := values.Values()
	obtainValues := runtime.New(0, 0, input)
	response, err := manager.execute(runContext, *manager.config.Obtain, obtainValues)
	if err != nil {
		return fmt.Errorf("não consegui obter a autenticação (%s): %w", manager.config.Obtain.AggregationKey(), err)
	}
	if response.Status >= 400 {
		return fmt.Errorf("a requisição de autenticação respondeu %d; confira usuário, senha e caminho em 'autenticacao.obter'", response.Status)
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
	manager.values = obtained
	manager.obtained = true
	manager.obtidoEm = manager.clock.Now()
	manager.Obtains++
	return nil
}

func (manager *Manager) venceu() bool {
	if manager.config.RefreshAfter <= 0 {
		return false
	}
	return manager.clock.Now().Sub(manager.obtidoEm) >= manager.config.RefreshAfter
}
