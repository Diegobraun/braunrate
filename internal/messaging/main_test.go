package messaging_test

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// Os testes de mensageria so medem contra broker de verdade, entao sem broker
// eles se pulam. O buraco nao era o pulo: era o pulo passar por aprovacao. Se o
// servico de broker nao subisse no CI, estes doze testes se pulariam e o portao
// ficaria verde sem ter medido nada — que e como oito commits entraram sem gate.
//
// BRAUNRATE_EXIGE_BROKER nomeia as variaveis que aquele passo declarou ter. O
// portao liga uma lista por passo, porque os passos tem brokers diferentes.
const requireBroker = "BRAUNRATE_EXIGE_BROKER"

func TestMain(m *testing.M) {
	if missing := missingRequired(); len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "%s exige %s, e essas variáveis não existem.\n"+
			"O broker não subiu, e pular aqui seria dar o portão por cumprido sem ter medido nada.\n",
			requireBroker, strings.Join(missing, ", "))
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func missingRequired() []string {
	var missing []string
	for _, variable := range strings.Split(os.Getenv(requireBroker), ",") {
		variable = strings.TrimSpace(variable)
		if variable != "" && os.Getenv(variable) == "" {
			missing = append(missing, variable)
		}
	}
	return missing
}
