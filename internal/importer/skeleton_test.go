package importer_test

import (
	"strings"
	"testing"

	"github.com/Diegobraun/braunrate/internal/importer"
	"github.com/Diegobraun/braunrate/internal/scenario"

	_ "github.com/Diegobraun/braunrate/internal/protocol/amqp"
	_ "github.com/Diegobraun/braunrate/internal/protocol/graphql"
	_ "github.com/Diegobraun/braunrate/internal/protocol/http"
	_ "github.com/Diegobraun/braunrate/internal/protocol/kafka"
	_ "github.com/Diegobraun/braunrate/internal/protocol/wait"
)

// The skeleton is the only way in for someone starting from an empty folder,
// and it used to teach one protocol out of five. Whoever needed messaging threw
// the whole output away and wrote the file by hand, with a shape only the source
// or the repository examples show.
func TestSkeletonShowsEveryProtocolThatIsCompiledIn(t *testing.T) {
	skeleton := importer.Skeleton()

	for _, protocol := range []string{"http", "graphql", "kafka", "amqp", "await"} {
		if !strings.Contains(skeleton, protocol) {
			t.Errorf("o esqueleto não mostra a forma do passo %q", protocol)
		}
	}
}

// What the skeleton shows uncommented has to stay a scenario that runs: it is
// the first thing the person executes.
func TestSkeletonIsAValidScenario(t *testing.T) {
	if _, err := scenario.Parse([]byte(importer.Skeleton())); err != nil {
		t.Fatalf("o esqueleto que a ferramenta escreve não passa no próprio parser: %v", err)
	}
}

// A commented shape that does not parse teaches the wrong thing to exactly the
// person with no other reference. The first version of this block wrote "value"
// on the amqp step, which uses "body".
func TestCommentedProtocolShapesParse(t *testing.T) {
	document := `name: formas
target: 127.0.0.1:9092
data:
  subscribers: { generate: { id: uuid } }
load:
  profiles:
    - steady: { rate: 20/s, duration: 1m }
` + importer.ProtocolShapes()

	if _, err := scenario.Parse([]byte(document)); err != nil {
		t.Fatalf("a forma que o esqueleto ensina não passa no parser: %v", err)
	}
}
