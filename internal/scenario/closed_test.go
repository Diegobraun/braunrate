package scenario_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Diegobraun/braunrate/internal/scenario"
)

func parseLoad(t *testing.T, load string) (scenario.Spec, error) {
	t.Helper()
	return scenario.Parse([]byte(`
nome: x
alvo: http://127.0.0.1:8080
carga:
` + load + `
cenario:
  - http: GET /pedidos
`))
}

func TestClosedLoadIsReadWholeRegardlessOfKeyOrder(t *testing.T) {
	spec, err := parseLoad(t, "  usuarios: 200\n  duracao: 5m\n  intervalo_entre_iteracoes: 1s\n  modelo: fechado\n")
	if err != nil {
		t.Fatalf("cenário recusado: %v", err)
	}
	if !spec.Load.Closed() || spec.Load.Users != 200 || spec.Load.For != 5*time.Minute || spec.Load.ThinkTime != time.Second {
		t.Fatalf("carga lida errada: %+v", spec.Load)
	}
	if spec.Duration() != 5*time.Minute {
		t.Fatalf("duração do cenário fechado veio %s", spec.Duration())
	}
}

// Each mix has a right answer, and the message is where it gets taught: the two
// models are not two ways of writing the same thing.
func TestEachMixOfTheTwoModelsIsRefusedWithTheWayOut(t *testing.T) {
	cases := []struct {
		name     string
		load     string
		contains []string
	}{
		{
			"perfis dentro do fechado",
			"  modelo: fechado\n  usuarios: 10\n  duracao: 1m\n  perfis:\n    - patamar: { taxa: 10/s, durante: 1m }\n",
			[]string{"não usa 'perfis'", "consequência do tempo de resposta"},
		},
		{
			"usuários dentro do aberto",
			"  usuarios: 10\n  perfis:\n    - patamar: { taxa: 10/s, durante: 1m }\n",
			[]string{"só existe no modelo fechado", "taxa: 300/s"},
		},
		{
			"fechado sem usuários",
			"  modelo: fechado\n  duracao: 1m\n",
			[]string{"precisa de 'usuarios'", "usuarios: 200"},
		},
		{
			"fechado sem duração",
			"  modelo: fechado\n  usuarios: 10\n",
			[]string{"precisa de 'duracao'"},
		},
		{
			"usuários que não e número",
			"  modelo: fechado\n  usuarios: muitos\n  duracao: 1m\n",
			[]string{"inteiro maior que zero"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := parseLoad(t, c.load)
			if err == nil {
				t.Fatal("o cenário foi aceito")
			}
			for _, fragment := range c.contains {
				if !strings.Contains(err.Error(), fragment) {
					t.Fatalf("a mensagem não ensina %q: %v", fragment, err)
				}
			}
		})
	}
}

// The rate is the number the reader came for, and in this model it is the one
// number that cannot be promised — so it comes with what it depends on.
func TestValidateWarnsAboutTheClosedModelWithTheRateItWouldProduce(t *testing.T) {
	spec, err := parseLoad(t, "  modelo: fechado\n  usuarios: 200\n  duracao: 5m\n  intervalo_entre_iteracoes: 1s\n")
	if err != nil {
		t.Fatalf("cenário recusado: %v", err)
	}

	warning, closed := scenario.ClosedModelWarning(spec)
	if !closed {
		t.Fatal("o modelo fechado passou sem aviso")
	}
	for _, fragment := range []string{"182/s", "133/s", "67/s", "se o alvo travar"} {
		if !strings.Contains(warning, fragment) {
			t.Fatalf("o aviso não traz %q:\n%s", fragment, warning)
		}
	}

	open, err := parseLoad(t, "  perfis:\n    - patamar: { taxa: 10/s, durante: 1m }\n")
	if err != nil {
		t.Fatalf("cenário aberto recusado: %v", err)
	}
	if _, closed := scenario.ClosedModelWarning(open); closed {
		t.Fatal("o modelo aberto recebeu o aviso do fechado")
	}
}
