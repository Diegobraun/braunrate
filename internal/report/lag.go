package report

import (
	"fmt"

	"github.com/Diegobraun/braunrate/internal/protocol"
	"github.com/Diegobraun/braunrate/internal/texto"
)

// The phrasing of the lag lives here because the terminal and the HTML answer
// the same question from the same numbers: two wordings of one measurement is
// two readings of it.
func lagSentences(lag protocol.ConsumerLag) (headline, note string) {
	switch {
	case lag.Problem != "":
		return fmt.Sprintf("não consegui medir — %s", lag.Problem), ""
	case lag.Readings == 0:
		return "nenhuma leitura no período", ""
	}
	headline = fmt.Sprintf("no pior momento %s atrás; no fim, %s",
		messages(lag.Max), messages(lag.Final))
	// The measurement is the distance between the high watermark and the
	// committed offset. Why the distance grew — a consumer that could not keep
	// up, one that stopped, one that was rebalancing — is not in it, and the
	// sentence used to name the first of the three as if it had been checked.
	if lag.Final > 0 {
		note = "O consumidor terminou a execução para trás. O atraso diz a distância, não a causa: consumidor lento, parado ou em rebalanceamento produzem o mesmo número."
	}
	return headline, note
}

func messages(quantity int64) string {
	return thousands(quantity) + " " + texto.Pick(quantity, "mensagem", "mensagens")
}
