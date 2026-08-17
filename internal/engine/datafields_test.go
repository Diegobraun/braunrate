package engine_test

import (
	"strings"
	"testing"

	"github.com/Diegobraun/braunrate/internal/engine"
	"github.com/Diegobraun/braunrate/internal/scenario"
)

// A column that is not in the CSV interpolated to nothing: the request went out
// with a blank in the middle of the path and only the target's 404 said so.
func TestColumnThatIsNotInTheCSVIsRefusedBeforeTheRun(t *testing.T) {
	spec, err := scenario.Parse([]byte(`
name: x
target: http://127.0.0.1:8090
data:
  clientes: { file: clientes.csv, consume: circular }
load:
  profiles:
    - steady: { rate: 1/s, duration: 1s }
scenario:
  - http: GET /pessoas/${clientes.identificador}/limite
`))
	if err != nil {
		t.Fatalf("o cenário não deveria falhar na leitura: %v", err)
	}

	options := engine.DefaultOptions()
	options.DataRoot = "testdata"
	_, err = engine.New(spec, options)
	if err == nil {
		t.Fatal("coluna inexistente foi aceita: a requisição sai com um vazio no meio do caminho")
	}
	for _, expected := range []string{"identificador", "available fields", "id"} {
		if !strings.Contains(err.Error(), expected) {
			t.Errorf("a mensagem não diz o que existe: falta %q em\n%v", expected, err)
		}
	}
}

func TestColumnThatExistsKeepsWorking(t *testing.T) {
	spec, err := scenario.Parse([]byte(`
name: x
target: http://127.0.0.1:8090
data:
  clientes: { file: clientes.csv, consume: circular }
load:
  profiles:
    - steady: { rate: 1/s, duration: 1s }
scenario:
  - http: GET /pessoas/${clientes.id}/limite
`))
	if err != nil {
		t.Fatalf("o cenário não deveria falhar na leitura: %v", err)
	}
	options := engine.DefaultOptions()
	options.DataRoot = "testdata"
	if _, err := engine.New(spec, options); err != nil {
		t.Fatalf("coluna que existe foi recusada: %v", err)
	}
}
