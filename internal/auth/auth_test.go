package auth_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Diegobraun/braunrate/internal/auth"
	"github.com/Diegobraun/braunrate/internal/protocol"
	"github.com/Diegobraun/braunrate/internal/runtime"
	"github.com/Diegobraun/braunrate/internal/scenario"
)

type testClock struct{ now time.Time }

func (testClock *testClock) Now() time.Time { return testClock.now }

func (testClock *testClock) Advance(duration time.Duration) {
	testClock.now = testClock.now.Add(duration)
}

func tokenConfig(refreshAfter time.Duration) scenario.Auth {
	return scenario.Auth{
		Kind:         scenario.AuthToken,
		Obtain:       &scenario.Step{Name: "obter autenticação", Protocol: "http"},
		RefreshAfter: refreshAfter,
		Header:       "Authorization: Bearer ${token}",
	}
}

func TestTokenIsObtainedOnceAndReused(t *testing.T) {
	clock := &testClock{now: time.Unix(1_700_000_000, 0)}
	obtains := 0
	execute := func(_ context.Context, _ scenario.Step, values *runtime.Values) (protocol.Response, error) {
		obtains++
		values.Set("token", "token-de-teste")
		return protocol.Response{Status: 200}, nil
	}

	manager := auth.New(tokenConfig(25*time.Minute), execute, clock)

	for i := 0; i < 50; i++ {
		name, value, err := manager.Header(context.Background(), runtime.New(int64(i), 0, nil))
		if err != nil {
			t.Fatalf("iteração %d: %v", i, err)
		}
		if name != "Authorization" || value != "Bearer token-de-teste" {
			t.Fatalf("cabecalho = %q: %q", name, value)
		}
	}
	if obtains != 1 {
		t.Errorf("obtencoes = %d, esperado 1: o token e obtido uma vez, não por requisição", obtains)
	}
}

func TestTokenIsRefreshedWhenItExpires(t *testing.T) {
	clock := &testClock{now: time.Unix(1_700_000_000, 0)}
	obtains := 0
	execute := func(_ context.Context, _ scenario.Step, values *runtime.Values) (protocol.Response, error) {
		obtains++
		values.Set("token", "token-"+time.Duration(obtains).String())
		return protocol.Response{Status: 200}, nil
	}

	manager := auth.New(tokenConfig(25*time.Minute), execute, clock)

	if _, _, err := manager.Header(context.Background(), runtime.New(0, 0, nil)); err != nil {
		t.Fatalf("primeira obtencao: %v", err)
	}
	clock.Advance(24 * time.Minute)
	if _, _, err := manager.Header(context.Background(), runtime.New(1, 0, nil)); err != nil {
		t.Fatalf("antes de vencer: %v", err)
	}
	if obtains != 1 {
		t.Fatalf("obtencoes = %d antes de vencer, esperado 1", obtains)
	}

	clock.Advance(2 * time.Minute)
	if _, _, err := manager.Header(context.Background(), runtime.New(2, 0, nil)); err != nil {
		t.Fatalf("após vencer: %v", err)
	}
	if obtains != 2 {
		t.Errorf("obtencoes = %d após vencer, esperado 2", obtains)
	}
}

func TestAuthFailureExplainsWhatToCheck(t *testing.T) {
	clock := &testClock{now: time.Unix(1_700_000_000, 0)}
	execute := func(context.Context, scenario.Step, *runtime.Values) (protocol.Response, error) {
		return protocol.Response{Status: 401}, nil
	}

	manager := auth.New(tokenConfig(0), execute, clock)
	_, _, err := manager.Header(context.Background(), runtime.New(0, 0, nil))
	if err == nil {
		t.Fatal("esperava erro")
	}
	for _, fragment := range []string{"401", "usuário", "senha", "autenticacao.obter"} {
		if !strings.Contains(err.Error(), fragment) {
			t.Errorf("mensagem %q não menciona %q", err.Error(), fragment)
		}
	}
}

func TestBasicAuthMakesNoRequest(t *testing.T) {
	clock := &testClock{now: time.Unix(1_700_000_000, 0)}
	execute := func(context.Context, scenario.Step, *runtime.Values) (protocol.Response, error) {
		t.Fatal("autenticação básica não deveria fazer requisição")
		return protocol.Response{}, nil
	}

	config := scenario.Auth{Kind: scenario.AuthBasic, User: "ana", Password: "${SENHA}"}
	manager := auth.New(config, execute, clock)

	values := runtime.New(0, 0, map[string]string{"SENHA": "segredo"})
	name, value, err := manager.Header(context.Background(), values)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if name != "Authorization" || value != "Basic YW5hOnNlZ3JlZG8=" {
		t.Errorf("cabecalho = %q: %q", name, value)
	}
}
