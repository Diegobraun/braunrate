package scenario

import (
	"fmt"
	"strings"

	"github.com/Diegobraun/braunrate/internal/protocol"
)

// A step that publishes to or reads from a broker fails at the first iteration
// when no address exists anywhere, and the message that explains it only shows
// up in `debug`. The three facts that prove the impossibility are in the file:
// the step declares no address, the messaging block has none for that
// technology, and the target is of another technology. Saying "cenario valido"
// with those three in hand is the validation approving what it can already
// read — the same rule already applied to an undeclared variable.
func checkBrokers(spec *Spec) []string {
	var problems []string
	said := map[string]bool{}
	for _, step := range spec.Steps {
		config, needs := step.Config.(protocol.WithBrokers)
		if !needs {
			continue
		}
		technology := config.BrokerTechnology()
		if technology == "" || said[technology] {
			continue
		}
		if len(config.DeclaredBrokers()) > 0 {
			continue
		}
		if broker := spec.Messaging.BrokerFor(technology); broker != nil && len(broker.Addresses) > 0 {
			continue
		}
		if brokerTarget(spec.Target) {
			continue
		}
		said[technology] = true
		problems = append(problems, fmt.Sprintf(
			"o passo %q fala com %s e nao existe endereco de broker em lugar nenhum: nem no passo, nem em 'mensageria', e o alvo do cenario e %q\n"+
				"    declare:  mensageria:\n"+
				"                %s:\n"+
				"                  brokers: [%s.homolog:9092]",
			step.Name, technology, spec.Target, technology, technology))
	}
	return problems
}

// The runtime accepts the target as the broker address whenever it is not an
// HTTP one, scheme or no scheme — "127.0.0.1:9092" is what people paste from a
// docker-compose. This mirrors that rule instead of guessing a stricter one:
// refusing here what the run would accept is worse than not refusing at all.
func brokerTarget(target string) bool {
	if target == "" {
		return false
	}
	return !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://")
}
