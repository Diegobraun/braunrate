package data_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/Diegobraun/braunrate/internal/data"
	"github.com/Diegobraun/braunrate/internal/engine"
	_ "github.com/Diegobraun/braunrate/internal/protocol/http"
	"github.com/Diegobraun/braunrate/internal/scenario"
)

func value(t *testing.T, generator scenario.Generator) string {
	t.Helper()
	source := scenario.DataSource{Name: "pedidos", Seed: 3, Fields: map[string]scenario.Generator{"campo": generator}}
	open, err := data.Open(source, "")
	if err != nil {
		t.Fatalf("abrir a fonte falhou: %v", err)
	}
	record, err := open.Next(0)
	if err != nil {
		t.Fatalf("gerar falhou: %v", err)
	}
	return record["pedidos.campo"]
}

func TestPatternFillsOnlyItsPlaceholders(t *testing.T) {
	generated := value(t, scenario.Generator{Recipe: "padrao", Format: "PED-######/@@"})

	if !regexp.MustCompile(`^PED-\d{6}/[A-Z]{2}$`).MatchString(generated) {
		t.Fatalf("formato nao respeitado: %q", generated)
	}
}

// An invalid document makes the target refuse every request with a validation
// error, and the run measures the rejection path instead of the work.
func TestGeneratedDocumentsHaveValidCheckDigits(t *testing.T) {
	cases := []struct {
		recipe  string
		length  int
		weights [][]int
	}{
		{"cpf", 11, [][]int{{10, 9, 8, 7, 6, 5, 4, 3, 2}, {11, 10, 9, 8, 7, 6, 5, 4, 3, 2}}},
		{"cnpj", 14, [][]int{{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}, {6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}}},
	}

	for _, c := range cases {
		t.Run(c.recipe, func(t *testing.T) {
			source := scenario.DataSource{Name: "clientes", Seed: 11,
				Fields: map[string]scenario.Generator{"doc": {Recipe: c.recipe}}}
			open, err := data.Open(source, "")
			if err != nil {
				t.Fatalf("abrir falhou: %v", err)
			}
			for iteration := 0; iteration < 200; iteration++ {
				record, err := open.Next(int64(iteration))
				if err != nil {
					t.Fatalf("gerar falhou: %v", err)
				}
				document := record["clientes.doc"]
				if len(document) != c.length {
					t.Fatalf("%s com %d digitos: %q", c.recipe, len(document), document)
				}
				if !validCheckDigits(t, document, c.weights) {
					t.Fatalf("%s com digito verificador invalido: %q", c.recipe, document)
				}
			}
		})
	}
}

func validCheckDigits(t *testing.T, document string, weights [][]int) bool {
	t.Helper()
	digits := make([]int, 0, len(document))
	for _, character := range document {
		digit, err := strconv.Atoi(string(character))
		if err != nil {
			return false
		}
		digits = append(digits, digit)
	}
	for offset, weight := range weights {
		position := len(digits) - 2 + offset
		total := 0
		for index, factor := range weight {
			total += digits[index] * factor
		}
		expected := 0
		if remainder := total % 11; remainder >= 2 {
			expected = 11 - remainder
		}
		if digits[position] != expected {
			return false
		}
	}
	return true
}

// recordingTarget answers the two steps of the journey and keeps the header in
// arrival order. Debug runs one user, one iteration, steps in sequence, so the
// order is deterministic and the pairing needs no correlation of its own.
func recordingTarget(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	var mu sync.Mutex
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Header.Get("X-Idempotency-Key"))
		mu.Unlock()
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
		}
		_, _ = fmt.Fprint(w, `{"id":"1"}`)
	}))
	t.Cleanup(server.Close)
	return server, &seen
}

func runTwoIterations(t *testing.T, generated string) []string {
	t.Helper()
	server, seen := recordingTarget(t)

	spec, err := scenario.Parse([]byte(fmt.Sprintf(`
nome: Idempotencia
alvo: %s

dados:
  pagamento:
    gerar:
      transactionId: %s

carga:
  perfis:
    - patamar: { taxa: 1/s, durante: 1s }

cenario:
  - http: { metodo: POST, caminho: "/pedidos", cabecalhos: { X-Idempotency-Key: "${pagamento.transactionId}" } }
    nome: criar pedido
    verificar: { status: 201 }

  - http: { metodo: GET, caminho: "/pedidos/1", cabecalhos: { X-Idempotency-Key: "${pagamento.transactionId}" } }
    nome: consultar pedido
    verificar: { status: 200 }
`, server.URL, generated)))
	if err != nil {
		t.Fatalf("cenario invalido: %v", err)
	}

	executor, err := engine.New(spec, engine.DefaultOptions())
	if err != nil {
		t.Fatalf("motor nao subiu: %v", err)
	}
	for iteration := 0; iteration < 2; iteration++ {
		observations, _, err := executor.Debug(context.Background())
		if err != nil {
			t.Fatalf("iteracao %d falhou: %v", iteration, err)
		}
		if len(observations) != 2 {
			t.Fatalf("iteracao %d parou no passo %d", iteration, len(observations))
		}
	}
	if len(*seen) != 4 {
		t.Fatalf("esperava 4 requisicoes, chegaram %d", len(*seen))
	}
	return *seen
}

// The idempotency case: the same transactionId has to reach both requests of
// one journey, and a new one has to reach the next journey. Both sides are
// checked here because either one alone would let the wrong default through.
func TestGeneratedValueIsStableWithinTheIterationAndNewInTheNext(t *testing.T) {
	seen := runTwoIterations(t, "uuid")

	if seen[0] != seen[1] {
		t.Fatalf("a mesma iteracao usou dois valores: %q e %q — chave de idempotencia quebrada", seen[0], seen[1])
	}
	if seen[2] != seen[3] {
		t.Fatalf("a segunda iteracao usou dois valores: %q e %q", seen[2], seen[3])
	}
	if seen[0] == seen[2] {
		t.Fatalf("as duas iteracoes usaram o mesmo valor %q — a chave nao renovou por jornada", seen[0])
	}
}

func TestNewPerUseIsExplicitAndChangesAtEveryOccurrence(t *testing.T) {
	seen := runTwoIterations(t, "{ tipo: uuid, novo_a_cada: uso }")

	if seen[0] == seen[1] {
		t.Fatalf("declarado 'novo_a_cada: uso' e os dois passos receberam o mesmo valor: %q", seen[0])
	}
	if seen[2] == seen[3] {
		t.Fatalf("declarado 'novo_a_cada: uso' e a segunda iteracao repetiu: %q", seen[2])
	}
}

func TestPatternWithoutFormatIsRefusedWithAnExample(t *testing.T) {
	_, err := scenario.Parse([]byte(`
nome: x
alvo: http://127.0.0.1:1
dados:
  pedidos:
    gerar:
      referencia: { tipo: padrao }
carga:
  perfis:
    - patamar: { taxa: 1/s, durante: 1s }
cenario:
  - http: GET /
`))

	if err == nil || !strings.Contains(err.Error(), "formato") || !strings.Contains(err.Error(), "PED-######") {
		t.Fatalf("esperava erro ensinando o formato, recebeu: %v", err)
	}
}
