package report

import (
	"io"
	"strings"

	"github.com/Diegobraun/braunrate/internal/report/comparison"
)

func Comparison(out io.Writer, c comparison.Comparison) error {
	lines := &lineWriter{out: out}
	write := lines.writef

	write("")
	write("%s", c.Sentence)
	write("")
	write("Comparando")
	write("  antes:  %s contra %s, em %s", c.Before.Spec, c.Before.Target, c.Before.Start)
	write("  depois: %s contra %s, em %s", c.After.Spec, c.After.Target, c.After.Start)
	write("")

	if c.Comparable {
		write("A jornada inteira")
		write("  %s", c.Journey.Sentence)
		write("")

		write("Por passo")
		write("  %-26s %11s %11s %16s", "passo", "95% antes", "95% depois", "variacao")
		for _, step := range c.Steps {
			note := ""
			switch {
			case step.New:
				note = "  (passo novo)"
			case step.Vanished:
				note = "  (nao existe mais)"
			}
			write("  %-26s %11s %11s %16s%s", trim(step.Step, 26),
				milliseconds(step.P95.Before), milliseconds(step.P95.After),
				change(step.P95), note)
		}
		write("")
		write("Erros")
		write("  %s", c.Error.Sentence)
		write("")
	}

	write("O que pode explicar a diferenca sem ser o servico")
	if len(c.Caveats) == 0 {
		write("  %s", noCaveatSentence)
	}
	for _, caveat := range c.Caveats {
		if caveat.Blocking {
			write("  - %s (isso sozinho explica a diferenca)", caveat.Text)
			continue
		}
		write("  - %s", caveat.Text)
	}
	write("  Duas execucoes nao dao intervalo de confianca: variacao abaixo de %.0f%% e tratada como ruido.", comparison.AcceptedNoise*100)
	write("")
	return lines.err
}

func change(difference comparison.Difference) string {
	if difference.Direction == comparison.DirectionSame {
		return "ruido"
	}
	magnitude := strings.Replace(comparison.Magnitude(difference), " vezes", "x", 1)
	if difference.Direction == comparison.DirectionBetter {
		return magnitude + " melhor"
	}
	return magnitude + " pior"
}

// "Nada" was an absolute claim over five fields it had checked. Replacing the
// whole CSV between two runs changed the p95 by 15x and the comparison still
// said nothing but the service could explain it.
const noCaveatSentence = "Nada do que da para comparar: cenario, alvo, maquina, plano de carga e versao sao os mesmos. O conteudo dos arquivos de dados nao entra nesta lista — se ele mudou entre as duas, a diferenca pode ser dele."
