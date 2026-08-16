package scenario

import (
	"strings"
	"testing"

	_ "github.com/Diegobraun/braunrate/internal/protocol/amqp"
	_ "github.com/Diegobraun/braunrate/internal/protocol/graphql"
	_ "github.com/Diegobraun/braunrate/internal/protocol/http"
	_ "github.com/Diegobraun/braunrate/internal/protocol/kafka"
	_ "github.com/Diegobraun/braunrate/internal/protocol/wait"
)

// A scenario error is almost always made by someone learning the format: the
// message has to show the right shape, not just refuse the wrong one.
func TestErrorMessagesShowTheRightForm(t *testing.T) {
	testCases := []struct {
		name     string
		content  string
		contains []string
	}{
		{
			name: "perfil escrito como mapa em vez de lista",
			content: `name: teste
target: http://127.0.0.1:8080
load:
  profiles:
    steady: { rate: 300/s, duration: 5m }
scenario:
  - http: GET /pedidos/1
`,
			contains: []string{"profiles has to be a list", "- steady: { rate: 300/s, duration: 5m }"},
		},
		{
			name: "duração no formato de outra ferramenta",
			content: `name: teste
target: http://127.0.0.1:8080
load:
  profiles:
    - steady: { rate: 300/s, duration: 5 minutos }
scenario:
  - http: GET /pedidos/1
`,
			contains: []string{"invalid duration", "30s, 5m, 1h30m"},
		},
		{
			name: "chave de carga com erro de digitacao",
			content: `name: teste
target: http://127.0.0.1:8080
load:
  perfil:
    - steady: { rate: 300/s, duration: 5m }
scenario:
  - http: GET /pedidos/1
`,
			contains: []string{"unknown key in load", "profiles"},
		},
		{
			name: "tipo de perfil inventado",
			content: `name: teste
target: http://127.0.0.1:8080
load:
  profiles:
    - degrau: { rate: 300/s, duration: 5m }
scenario:
  - http: GET /pedidos/1
`,
			contains: []string{"unknown profile kind", "steady"},
		},
		{
			name: "autenticação por token sem o bloco obter",
			content: `name: teste
target: http://127.0.0.1:8080
auth:
  type: token
load:
  profiles:
    - steady: { rate: 300/s, duration: 5m }
scenario:
  - http: GET /pedidos/1
`,
			contains: []string{"the 'obtain' block", "capture: { token: $.access_token }"},
		},
		{
			name: "slo escrito como mapa em vez de lista",
			content: `name: teste
target: http://127.0.0.1:8080
load:
  profiles:
    - steady: { rate: 300/s, duration: 5m }
scenario:
  - http: GET /pedidos/1
    name: consultar pedido
slo:
  consultar pedido: { p95: < 150ms }
`,
			contains: []string{"slo has to be a list", "p95"},
		},
		{
			name: "métrica de slo que não existe",
			content: `name: teste
target: http://127.0.0.1:8080
load:
  profiles:
    - steady: { rate: 300/s, duration: 5m }
scenario:
  - http: GET /pedidos/1
    name: consultar pedido
slo:
  - consultar pedido: { média: < 150ms }
`,
			contains: []string{"unknown slo metric", "p95"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := Parse([]byte(testCase.content))
			if err == nil {
				t.Fatal("esperava erro e o cenário carregou")
			}
			message := err.Error()
			if !strings.Contains(message, "line") {
				t.Errorf("a mensagem precisa apontar a linha:\n%s", message)
			}
			for _, fragment := range testCase.contains {
				if !strings.Contains(message, fragment) {
					t.Errorf("a mensagem não mostra %q:\n%s", fragment, message)
				}
			}
		})
	}
}

// The curl importer masks a password before writing the file. Hand-written, the
// same password in 'variaveis' was accepted and went to the repository.
func TestLiteralCredentialInVariablesIsRefused(t *testing.T) {
	for _, name := range []string{"senha", "password", "token", "api_key", "client_secret"} {
		_, err := Parse([]byte("name: x\ntarget: http://a\nvariables:\n  " + name + ": valor-de-verdade\nload:\n  profiles:\n    - steady: { rate: 1/s, duration: 1s }\nscenario:\n  - http: GET /\n"))
		if err == nil {
			t.Errorf("%q literal foi aceita e vai para o repositorio", name)
			continue
		}
		if !strings.Contains(err.Error(), "${"+strings.ToUpper(name)+"}") {
			t.Errorf("a mensagem de %q não ensina a forma certa: %v", name, err)
		}
	}
}

func TestCredentialFromTheEnvironmentIsAccepted(t *testing.T) {
	_, err := Parse([]byte("name: x\ntarget: http://a\nvariables:\n  password: \"${SENHA}\"\nload:\n  profiles:\n    - steady: { rate: 1/s, duration: 1s }\nscenario:\n  - http: GET /\n"))
	if err != nil {
		t.Fatalf("senha vinda do ambiente foi recusada: %v", err)
	}
}

// A variable whose name is not a credential keeps working with a literal: the
// rule is about secrets, not about writing values in the file.
func TestOrdinaryVariableKeepsAcceptingALiteral(t *testing.T) {
	_, err := Parse([]byte("name: x\ntarget: http://a\nvariables:\n  user: ana\nload:\n  profiles:\n    - steady: { rate: 1/s, duration: 1s }\nscenario:\n  - http: GET /\n"))
	if err != nil {
		t.Fatalf("variável comum foi recusada: %v", err)
	}
}
