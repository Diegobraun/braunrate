package scenario_test

import (
	"strings"
	"testing"

	"github.com/Diegobraun/braunrate/internal/scenario"
)

// This was the only path in the product that answered in English, with no file
// and no column. It is also the most common mistake there is: ${ and } are
// exactly the characters an inline map uses.
func TestYAMLSyntaxErrorAnswersInPortugueseWithThePositionAndTheFix(t *testing.T) {
	cases := []struct {
		name     string
		content  string
		fragment string
		line     int
	}{
		{
			name: "variavel sem aspas dentro de mapa em linha",
			content: "nome: x\nalvo: 127.0.0.1:9092\ncarga:\n  perfis:\n    - patamar: { taxa: 1/s, durante: 1s }\ncenario:\n" +
				"  - kafka: { topico: pedidos, chave: ${pedidos.id} }\n",
			fragment: `chave: "${pedidos.id}"`,
			line:     7,
		},
		{
			name: "caminho json com colchete dentro de mapa em linha",
			content: "nome: x\nalvo: http://a\ncarga:\n  perfis:\n    - patamar: { taxa: 1/s, durante: 1s }\ncenario:\n" +
				"  - http: GET /faturas\n    captura: { faturaId: $.itens[0].id }\n",
			fragment: `"$.itens[0].id"`,
			line:     8,
		},
		{
			name:     "tabulacao no lugar de espaco",
			content:  "nome: x\nalvo: http://a\ncarga:\n\tperfis: []\n",
			fragment: "tabulacao",
			line:     4,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := scenario.Parse([]byte(testCase.content))
			if err == nil {
				t.Fatal("o YAML quebrado foi aceito")
			}
			if strings.Contains(err.Error(), "yaml:") || strings.Contains(err.Error(), "did not find") {
				t.Fatalf("o erro saiu cru, em ingles: %v", err)
			}
			if !strings.Contains(err.Error(), testCase.fragment) {
				t.Fatalf("a mensagem nao mostra a forma certa (%q): %v", testCase.fragment, err)
			}
			position, is := err.(scenario.ScenarioError)
			if !is {
				t.Fatalf("o erro nao carrega posicao: %#v", err)
			}
			if position.Line != testCase.line {
				t.Fatalf("apontou a linha %d, o problema esta na %d: %v", position.Line, testCase.line, err)
			}
		})
	}
}

// Knowing that "perfis" exists does not teach that "perfis" is a list of maps
// with a profile kind inside. A3 and A5 of the audit cost two edits each
// because the message listed names and stopped there.
func TestUnknownKeyShowsTheShapeAndNotOnlyTheNames(t *testing.T) {
	cases := []struct {
		name    string
		content string
		shape   string
	}{
		{
			name:    "carga",
			content: "nome: x\nalvo: http://a\ncarga:\n  taxa: 100/s\ncenario:\n  - http: GET /\n",
			shape:   "patamar: { taxa: 200/s, durante: 5m }",
		},
		{
			name: "autenticacao",
			content: "nome: x\nalvo: http://a\nautenticacao:\n  tipo: token\n  url: /auth\n" +
				"carga:\n  perfis:\n    - patamar: { taxa: 1/s, durante: 1s }\ncenario:\n  - http: GET /\n",
			shape: "captura: { token: \"$.access_token\" }",
		},
		{
			name:    "chave de topo",
			content: "nome: x\nalvo: http://a\nvelocidade: 10\ncenario:\n  - http: GET /\n",
			shape:   "alvo: http://127.0.0.1:8080",
		},
		{
			name: "perfil",
			content: "nome: x\nalvo: http://a\ncarga:\n  perfis:\n    - patamar: { velocidade: 1/s, durante: 1s }\n" +
				"cenario:\n  - http: GET /\n",
			shape: "rampa: { de: 10/s, ate: 200/s, durante: 30s }",
		},
		{
			name: "passo",
			content: "nome: x\nalvo: http://a\ncarga:\n  perfis:\n    - patamar: { taxa: 1/s, durante: 1s }\n" +
				"cenario:\n  - http: GET /\n    conferir: { status: 200 }\n",
			shape: "verificar: { status: 200 }",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := scenario.Parse([]byte(testCase.content))
			if err == nil {
				t.Fatal("a chave desconhecida foi aceita")
			}
			if !strings.Contains(err.Error(), "disponiveis:") {
				t.Fatalf("a mensagem nao lista as chaves validas: %v", err)
			}
			if !strings.Contains(err.Error(), testCase.shape) {
				t.Fatalf("a mensagem nao mostra a forma (%q): %v", testCase.shape, err)
			}
		})
	}
}
