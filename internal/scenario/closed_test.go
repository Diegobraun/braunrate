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
name: x
target: http://127.0.0.1:8080
load:
` + load + `
scenario:
  - http: GET /pedidos
`))
}

func TestClosedLoadIsReadWholeRegardlessOfKeyOrder(t *testing.T) {
	spec, err := parseLoad(t, "  users: 200\n  duration: 5m\n  thinkTime: 1s\n  model: closed\n")
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
			"  model: closed\n  users: 10\n  duration: 1m\n  profiles:\n    - steady: { rate: 10/s, duration: 1m }\n",
			[]string{"does not use 'profiles'", "a consequence of how fast the target answers"},
		},
		{
			"usuários dentro do aberto",
			"  users: 10\n  profiles:\n    - steady: { rate: 10/s, duration: 1m }\n",
			[]string{"only exists in the closed model", "rate: 300/s"},
		},
		{
			"fechado sem usuários",
			"  model: closed\n  duration: 1m\n",
			[]string{"needs 'users'", "users: 200"},
		},
		{
			"fechado sem duração",
			"  model: closed\n  users: 10\n",
			[]string{"needs 'duration'"},
		},
		{
			"usuários que não e número",
			"  model: closed\n  users: muitos\n  duration: 1m\n",
			[]string{"integer greater than zero"},
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
	spec, err := parseLoad(t, "  model: closed\n  users: 200\n  duration: 5m\n  thinkTime: 1s\n")
	if err != nil {
		t.Fatalf("cenário recusado: %v", err)
	}

	warning, closed := scenario.ClosedModelWarning(spec)
	if !closed {
		t.Fatal("o modelo fechado passou sem aviso")
	}
	for _, fragment := range []string{"182/s", "133/s", "67/s", "if the target freezes"} {
		if !strings.Contains(warning, fragment) {
			t.Fatalf("o aviso não traz %q:\n%s", fragment, warning)
		}
	}

	open, err := parseLoad(t, "  profiles:\n    - steady: { rate: 10/s, duration: 1m }\n")
	if err != nil {
		t.Fatalf("cenário aberto recusado: %v", err)
	}
	if _, closed := scenario.ClosedModelWarning(open); closed {
		t.Fatal("o modelo aberto recebeu o aviso do fechado")
	}
}
