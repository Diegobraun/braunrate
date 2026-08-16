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

// O token e obtido uma vez e renovado quando vence, nunca por requisicao: o
// autor declara a renovacao e o motor cuida de quando ela acontece.
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

	// So o que a requisicao de autenticacao produziu fica guardado. Guardar o
	// contexto inteiro congelaria os dados da primeira iteracao e os reinjetaria
	// em todas as outras: toda a carga cairia sobre a primeira linha do CSV, com
	// o relatorio afirmando variedade que nao existiu.
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
