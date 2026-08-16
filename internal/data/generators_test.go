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
		t.Fatalf("formato não respeitado: %q", generated)
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
					t.Fatalf("%s com digito verificador inválido: %q", c.recipe, document)
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
name: Idempotencia
target: %s

data:
  pagamento:
    generate:
      transactionId: %s

load:
  profiles:
    - steady: { rate: 1/s, duration: 1s }

scenario:
  - http: { method: POST, path: "/pedidos", headers: { X-Idempotency-Key: "${pagamento.transactionId}" } }
    name: criar pedido
    expect: { status: 201 }

  - http: { method: GET, path: "/pedidos/1", headers: { X-Idempotency-Key: "${pagamento.transactionId}" } }
    name: consultar pedido
    expect: { status: 200 }
`, server.URL, generated)))
	if err != nil {
		t.Fatalf("cenário inválido: %v", err)
	}

	executor, err := engine.New(spec, engine.DefaultOptions())
	if err != nil {
		t.Fatalf("motor não subiu: %v", err)
	}
	for iteration := 0; iteration < 2; iteration++ {
		observations, _, err := executor.Debug(context.Background())
		if err != nil {
			t.Fatalf("iteração %d falhou: %v", iteration, err)
		}
		if len(observations) != 2 {
			t.Fatalf("iteração %d parou no passo %d", iteration, len(observations))
		}
	}
	if len(*seen) != 4 {
		t.Fatalf("esperava 4 requisições, chegaram %d", len(*seen))
	}
	return *seen
}

// The idempotency case: the same transactionId has to reach both requests of
// one journey, and a new one has to reach the next journey. Both sides are
// checked here because either one alone would let the wrong default through.
func TestGeneratedValueIsStableWithinTheIterationAndNewInTheNext(t *testing.T) {
	seen := runTwoIterations(t, "uuid")

	if seen[0] != seen[1] {
		t.Fatalf("a mesma iteração usou dois valores: %q e %q — chave de idempotência quebrada", seen[0], seen[1])
	}
	if seen[2] != seen[3] {
		t.Fatalf("a segunda iteração usou dois valores: %q e %q", seen[2], seen[3])
	}
	if seen[0] == seen[2] {
		t.Fatalf("as duas iterações usaram o mesmo valor %q — a chave não renovou por jornada", seen[0])
	}
}

func TestNewPerUseIsExplicitAndChangesAtEveryOccurrence(t *testing.T) {
	seen := runTwoIterations(t, "{ type: uuid, newEvery: use }")

	if seen[0] == seen[1] {
		t.Fatalf("declarado 'novo_a_cada: uso' e os dois passos receberam o mesmo value: %q", seen[0])
	}
	if seen[2] == seen[3] {
		t.Fatalf("declarado 'novo_a_cada: uso' e a segunda iteração repetiu: %q", seen[2])
	}
}

func TestPatternWithoutFormatIsRefusedWithAnExample(t *testing.T) {
	_, err := scenario.Parse([]byte(`
name: x
target: http://127.0.0.1:1
data:
  pedidos:
    generate:
      referencia: { type: padrao }
load:
  profiles:
    - steady: { rate: 1/s, duration: 1s }
scenario:
  - http: GET /
`))

	if err == nil || !strings.Contains(err.Error(), "formato") || !strings.Contains(err.Error(), "PED-######") {
		t.Fatalf("esperava erro ensinando o formato, recebeu: %v", err)
	}
}

// The pattern has two shapes and only one of them was read. `padrao(BR-######)`
// produced an empty string in silence: the request went out with a blank field,
// the target answered 404, and nothing connected the two — the failure this
// tool exists to catch, coming from the tool itself.
func TestInlinePatternProducesTheValueAndNotSilence(t *testing.T) {
	generated := value(t, scenario.Generator{Recipe: "padrao(BR-######)"})

	if generated == "" {
		t.Fatal("padrao(BR-######) devolveu vazio: valor em branco sai para o alvo sem ninguém ser avisado")
	}
	if !regexp.MustCompile(`^BR-\d{6}$`).MatchString(generated) {
		t.Fatalf("formato não respeitado: %q", generated)
	}
}

// The two shapes are the same generator, and a value that changes with the
// shape used to declare it would be a second behaviour hiding behind one name.
func TestBothShapesOfThePatternAgree(t *testing.T) {
	inline := value(t, scenario.Generator{Recipe: "padrao(BR-######)"})
	declared := value(t, scenario.Generator{Recipe: "padrao", Format: "BR-######"})

	if inline != declared {
		t.Fatalf("mesma semente e mesmo formato deram valores diferentes: %q e %q", inline, declared)
	}
}

func TestInlinePatternWithoutFormatSaysWhatIsMissing(t *testing.T) {
	source := scenario.DataSource{Name: "pedidos", Seed: 3, Fields: map[string]scenario.Generator{
		"campo": {Recipe: "padrao()"},
	}}
	open, err := data.Open(source, "")
	if err == nil {
		_, err = open.Next(0)
	}
	if err == nil {
		t.Fatal("padrão sem formato passou calado")
	}
	if !strings.Contains(err.Error(), "formato") || !strings.Contains(err.Error(), "######") {
		t.Fatalf("o erro não ensina o format: %v", err)
	}
}
