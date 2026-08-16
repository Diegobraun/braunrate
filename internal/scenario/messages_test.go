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
			content: `nome: teste
alvo: http://127.0.0.1:8080
carga:
  perfis:
    patamar: { taxa: 300/s, durante: 5m }
cenario:
  - http: GET /pedidos/1
`,
			contains: []string{"perfis precisa ser uma lista", "- patamar: { taxa: 300/s, durante: 5m }"},
		},
		{
			name: "duracao no formato de outra ferramenta",
			content: `nome: teste
alvo: http://127.0.0.1:8080
carga:
  perfis:
    - patamar: { taxa: 300/s, durante: 5 minutos }
cenario:
  - http: GET /pedidos/1
`,
			contains: []string{"duracao invalida", "30s, 5m, 1h30m"},
		},
		{
			name: "chave de carga com erro de digitacao",
			content: `nome: teste
alvo: http://127.0.0.1:8080
carga:
  perfil:
    - patamar: { taxa: 300/s, durante: 5m }
cenario:
  - http: GET /pedidos/1
`,
			contains: []string{"chave desconhecida em carga", "perfis"},
		},
		{
			name: "tipo de perfil inventado",
			content: `nome: teste
alvo: http://127.0.0.1:8080
carga:
  perfis:
    - degrau: { taxa: 300/s, durante: 5m }
cenario:
  - http: GET /pedidos/1
`,
			contains: []string{"tipo de perfil desconhecido", "patamar"},
		},
		{
			name: "autenticacao por token sem o bloco obter",
			content: `nome: teste
alvo: http://127.0.0.1:8080
autenticacao:
  tipo: token
carga:
  perfis:
    - patamar: { taxa: 300/s, durante: 5m }
cenario:
  - http: GET /pedidos/1
`,
			contains: []string{"bloco 'obter'", "captura: { token: $.access_token }"},
		},
		{
			name: "slo escrito como mapa em vez de lista",
			content: `nome: teste
alvo: http://127.0.0.1:8080
carga:
  perfis:
    - patamar: { taxa: 300/s, durante: 5m }
cenario:
  - http: GET /pedidos/1
    nome: consultar pedido
slo:
  consultar pedido: { p95: < 150ms }
`,
			contains: []string{"slo precisa ser uma lista", "p95"},
		},
		{
			name: "metrica de slo que nao existe",
			content: `nome: teste
alvo: http://127.0.0.1:8080
carga:
  perfis:
    - patamar: { taxa: 300/s, durante: 5m }
cenario:
  - http: GET /pedidos/1
    nome: consultar pedido
slo:
  - consultar pedido: { media: < 150ms }
`,
			contains: []string{"metrica de slo desconhecida", "p95"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := Parse([]byte(testCase.content))
			if err == nil {
				t.Fatal("esperava erro e o cenario carregou")
			}
			message := err.Error()
			if !strings.Contains(message, "linha") {
				t.Errorf("a mensagem precisa apontar a linha:\n%s", message)
			}
			for _, fragment := range testCase.contains {
				if !strings.Contains(message, fragment) {
					t.Errorf("a mensagem nao mostra %q:\n%s", fragment, message)
				}
			}
		})
	}
}

// The curl importer masks a password before writing the file. Hand-written, the
// same password in 'variaveis' was accepted and went to the repository.
func TestLiteralCredentialInVariablesIsRefused(t *testing.T) {
	for _, name := range []string{"senha", "password", "token", "api_key", "client_secret"} {
		_, err := Parse([]byte("nome: x\nalvo: http://a\nvariaveis:\n  " + name + ": valor-de-verdade\ncarga:\n  perfis:\n    - patamar: { taxa: 1/s, durante: 1s }\ncenario:\n  - http: GET /\n"))
		if err == nil {
			t.Errorf("%q literal foi aceita e vai para o repositorio", name)
			continue
		}
		if !strings.Contains(err.Error(), "${"+strings.ToUpper(name)+"}") {
			t.Errorf("a mensagem de %q nao ensina a forma certa: %v", name, err)
		}
	}
}

func TestCredentialFromTheEnvironmentIsAccepted(t *testing.T) {
	_, err := Parse([]byte("nome: x\nalvo: http://a\nvariaveis:\n  senha: \"${SENHA}\"\ncarga:\n  perfis:\n    - patamar: { taxa: 1/s, durante: 1s }\ncenario:\n  - http: GET /\n"))
	if err != nil {
		t.Fatalf("senha vinda do ambiente foi recusada: %v", err)
	}
}

// A variable whose name is not a credential keeps working with a literal: the
// rule is about secrets, not about writing values in the file.
func TestOrdinaryVariableKeepsAcceptingALiteral(t *testing.T) {
	_, err := Parse([]byte("nome: x\nalvo: http://a\nvariaveis:\n  usuario: ana\ncarga:\n  perfis:\n    - patamar: { taxa: 1/s, durante: 1s }\ncenario:\n  - http: GET /\n"))
	if err != nil {
		t.Fatalf("variavel comum foi recusada: %v", err)
	}
}
