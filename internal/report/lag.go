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
		return fmt.Sprintf("nao consegui medir — %s", lag.Problem), ""
	case lag.Readings == 0:
		return "nenhuma leitura no periodo", ""
	}
	headline = fmt.Sprintf("no pior momento %s atras; no fim, %s",
		messages(lag.Max), messages(lag.Final))
	if lag.Final > 0 {
		note = "O consumidor terminou a execucao para tras: a fila cresceu mais rapido do que ele consumiu."
	}
	return headline, note
}

func messages(quantity int64) string {
	return thousands(quantity) + " " + texto.Pick(quantity, "mensagem", "mensagens")
}
