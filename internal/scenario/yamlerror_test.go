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
