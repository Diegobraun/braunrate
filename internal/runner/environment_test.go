package runner_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/Diegobraun/braunrate/internal/protocol/http"
	"github.com/Diegobraun/braunrate/internal/runner"
)

func scenarioFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cenario.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("nao consegui escrever o cenario: %v", err)
	}
	return path
}

const needsEnvironment = `
nome: x
alvo: http://127.0.0.1:8080

autenticacao:
  tipo: basica
  usuario: ana
  senha: "${SENHA_DA_API_QUE_NINGUEM_DEFINIU}"

carga:
  perfis:
    - patamar: { taxa: 1/s, durante: 1s }

cenario:
  - http: GET /pedidos/1
`

// A6 of the audit: the request went out with an empty password, the target
// answered 401, and nothing in the output connected the two.
func TestRunningWithoutTheEnvironmentVariableIsRefusedBeforeAnythingIsSent(t *testing.T) {
	path := scenarioFile(t, needsEnvironment)

	_, err := runner.Execute(context.Background(), path, runner.DefaultOptions("teste"))
	if err == nil {
		t.Fatal("a execucao comecou com a credencial vazia")
	}
	fault, is := err.(runner.Fault)
	if !is || fault.Exit != runner.ExitBadFile {
		t.Fatalf("codigo de saida errado para cenario que nao pode rodar: %#v", err)
	}
	for _, fragment := range []string{"SENHA_DA_API_QUE_NINGUEM_DEFINIU=...", "reserva"} {
		if !strings.Contains(err.Error(), fragment) {
			t.Fatalf("a mensagem nao ensina %q: %v", fragment, err)
		}
	}

	if _, err := runner.Debug(context.Background(), path, "teste"); err == nil {
		t.Fatal("a depuracao comecou com a credencial vazia")
	}
}

// Validation is about the file and often runs where the secret is not — on a
// laptop before committing, in a lint job without credentials. There it warns.
func TestValidationOnlyWarnsSoItStillWorksWithoutTheSecret(t *testing.T) {
	spec, plan, err := runner.Load(scenarioFile(t, needsEnvironment))
	if err != nil {
		t.Fatalf("a validacao recusou o cenario: %v", err)
	}

	lines := strings.Join(runner.Describe(spec, plan), "\n")
	if !strings.Contains(lines, "SENHA_DA_API_QUE_NINGUEM_DEFINIU") {
		t.Fatalf("a validacao nao avisou da variavel que falta:\n%s", lines)
	}
	if !strings.Contains(lines, "recusa") {
		t.Fatalf("o aviso nao diz o que vai acontecer na execucao:\n%s", lines)
	}
}

func TestDefinedVariableAndDeclaredFallbackBothPass(t *testing.T) {
	t.Setenv("SENHA_DA_API_QUE_NINGUEM_DEFINIU", "segredo")
	spec, _, err := runner.Load(scenarioFile(t, needsEnvironment))
	if err != nil {
		t.Fatalf("cenario recusado: %v", err)
	}
	if err := runner.RequireEnvironment(spec); err != nil {
		t.Fatalf("variavel definida ainda foi cobrada: %v", err)
	}

	withFallback := strings.Replace(needsEnvironment,
		`"${SENHA_DA_API_QUE_NINGUEM_DEFINIU}"`, `"${OUTRA_QUE_NAO_EXISTE:-segredo}"`, 1)
	spec, _, err = runner.Load(scenarioFile(t, withFallback))
	if err != nil {
		t.Fatalf("cenario com reserva recusado: %v", err)
	}
	if err := runner.RequireEnvironment(spec); err != nil {
		t.Fatalf("reserva declarada ainda foi cobrada: %v", err)
	}
}
