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
			name: "variável sem aspas dentro de mapa em linha",
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
			name:     "tabulacao no lugar de espaço",
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
				t.Fatalf("a mensagem não mostra a forma certa (%q): %v", testCase.fragment, err)
			}
			position, is := err.(scenario.ScenarioError)
			if !is {
				t.Fatalf("o erro não carrega posição: %#v", err)
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
			if !strings.Contains(err.Error(), "disponíveis:") {
				t.Fatalf("a mensagem não lista as chaves validas: %v", err)
			}
			if !strings.Contains(err.Error(), testCase.shape) {
				t.Fatalf("a mensagem não mostra a forma (%q): %v", testCase.shape, err)
			}
		})
	}
}

// A2 of the audit: the raw error from the operating system, in English, in a
// product where every other message is in Portuguese and says what to do next.
func TestMissingFileAnswersInPortugueseAndPointsAtTheNextStep(t *testing.T) {
	_, err := scenario.ParseFile("cenario-que-nao-existe.yaml")
	if err == nil {
		t.Fatal("arquivo inexistente foi aceito")
	}
	message := err.Error()
	if strings.Contains(message, "no such file") {
		t.Fatalf("o erro do sistema operacional saiu cru: %v", err)
	}
	for _, fragment := range []string{"não encontrei o arquivo", "braunrate new"} {
		if !strings.Contains(message, fragment) {
			t.Fatalf("a mensagem não traz %q: %v", fragment, err)
		}
	}
}

// A4 of the audit: "taxa" is not a typo of "rampa", and the fixed distance of
// three said it was. The tolerance grows with the word.
func TestSuggestionOnlyFiresForAPlausibleTypo(t *testing.T) {
	_, err := scenario.Parse([]byte("nome: x\nalvo: http://a\ncarga:\n  perfis:\n    - taxa: { taxa: 1/s, durante: 1s }\ncenario:\n  - http: GET /\n"))
	if err == nil {
		t.Fatal("o perfil desconhecido foi aceito")
	}
	if strings.Contains(err.Error(), "você quis dizer") {
		t.Fatalf("sugeriu para palavra sem relação: %v", err)
	}

	_, err = scenario.Parse([]byte("nome: x\nalvo: http://a\ncarga:\n  perfis:\n    - patamer: { taxa: 1/s, durante: 1s }\ncenario:\n  - http: GET /\n"))
	if err == nil || !strings.Contains(err.Error(), `você quis dizer "patamar"?`) {
		t.Fatalf("erro de digitacao de verdade deixou de ser sugerido: %v", err)
	}
}
