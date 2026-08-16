package metrics_test

import (
	"strings"
	"testing"

	"github.com/Diegobraun/braunrate/internal/metrics"
)

// Counting distinct bodies counts distinct ids. What the target branches on is
// the shape, and two bodies with different ids are one code path.
func TestBodiesThatDifferOnlyInValueHaveTheSameShape(t *testing.T) {
	first := metrics.BodyShape([]byte(`{"id":"a1","total":10.5}`))
	second := metrics.BodyShape([]byte(`{"id":"b2","total":99.9}`))
	if first != second {
		t.Fatalf("mesma forma virou duas:\n  %s\n  %s", first, second)
	}
}

func TestShapeSeparatesWhatTheTargetTreatsDifferently(t *testing.T) {
	base := metrics.BodyShape([]byte(`{"id":"a1","total":10}`))
	cases := map[string]string{
		"campo a mais":    `{"id":"a1","total":10,"cupom":"X"}`,
		"campo vazio":     `{"id":"a1","total":10,"cupom":""}`,
		"tipo diferente":  `{"id":"a1","total":"10"}`,
		"campo faltando":  `{"id":"a1"}`,
		"lista sem itens": `{"id":"a1","total":10,"itens":[]}`,
	}
	for name, body := range cases {
		if metrics.BodyShape([]byte(body)) == base {
			t.Errorf("%s deveria ser outra forma, e saiu igual", name)
		}
	}
}

// The order the keys come in is an accident of the JSON, not a difference the
// target sees.
func TestShapeDoesNotDependOnKeyOrder(t *testing.T) {
	first := metrics.BodyShape([]byte(`{"id":"a1","total":10}`))
	second := metrics.BodyShape([]byte(`{"total":10,"id":"a1"}`))
	if first != second {
		t.Fatalf("a ordem das chaves mudou a forma:\n  %s\n  %s", first, second)
	}
}

// The length of a list is not a code path; having items or not is.
func TestListLengthIsNotShapeButBeingEmptyIs(t *testing.T) {
	one := metrics.BodyShape([]byte(`{"itens":[{"sku":"a"}]}`))
	three := metrics.BodyShape([]byte(`{"itens":[{"sku":"a"},{"sku":"b"},{"sku":"c"}]}`))
	none := metrics.BodyShape([]byte(`{"itens":[]}`))

	if one != three {
		t.Fatalf("lista de 1 e de 3 são o mesmo caminho:\n  %s\n  %s", one, three)
	}
	if one == none {
		t.Fatalf("lista vazia e outro caminho, e saiu igual: %s", none)
	}
}

func TestBodyThatIsNotJSONStillHasAShape(t *testing.T) {
	if shape := metrics.BodyShape([]byte("id=a1&total=10")); shape == "" {
		t.Fatal("corpo que não e JSON ficou sem forma")
	}
	if shape := metrics.BodyShape([]byte("   ")); !strings.Contains(shape, "vazio") {
		t.Fatalf("corpo em branco precisa aparecer como vazio, e saiu %q", shape)
	}
}

// A body the scenario wrote as {} is a declaration. Warning about it filled the
// report of a healthy run with two warnings that had nothing to say, and a
// reader who learns to skip that block skips the real ones with it.
func TestBodyDeclaredEmptyIsNotAnEmptyField(t *testing.T) {
	shape := metrics.BodyShape([]byte(`{}`))
	if strings.Contains(shape, ":") {
		t.Fatalf("a forma do corpo vazio saiu com nome de campo em branco: %q", shape)
	}

	document := metrics.Document{Variety: []metrics.Variety{{
		Name: metrics.BodyShapeName + "abrir carrinho", Distinct: 1, Uses: 500,
		Shapes: []string{shape},
	}}}
	if warnings := metrics.VarietyWarnings(document.Variety); len(warnings) != 0 {
		t.Fatalf("corpo declarado como {} virou aviso: %+v", warnings)
	}
	if document.Variety[0].Notable() {
		t.Fatalf("corpo declarado como {} rendeu linha no relatório: %q", document.Variety[0].Sentence)
	}
}

// The accident still has to be caught: a field that exists and came blank is
// the shape of an interpolation that resolved to nothing.
func TestFieldThatCameBlankIsStillWarned(t *testing.T) {
	document := metrics.Document{Variety: []metrics.Variety{{
		Name: metrics.BodyShapeName + "criar pedido", Distinct: 1, Uses: 500,
		Shapes: []string{metrics.BodyShape([]byte(`{"id":"a1","cupom":""}`))},
	}}}
	warnings := metrics.VarietyWarnings(document.Variety)
	if len(warnings) != 1 {
		t.Fatalf("campo vazio deixou de ser avisado: %+v", warnings)
	}
	if !strings.Contains(warnings[0].Evidence, "cupom: vazio") {
		t.Fatalf("o aviso não nomeou o campo que veio vazio: %s", warnings[0].Evidence)
	}
}
