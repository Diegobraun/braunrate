package relatorio

import (
	"fmt"
	"io"
	"strings"

	"github.com/Diegobraun/braunrate/comparacao"
)

func Comparacao(saida io.Writer, c comparacao.Comparacao) {
	escrever := func(formato string, argumentos ...any) {
		fmt.Fprintf(saida, formato+"\n", argumentos...)
	}

	escrever("")
	escrever("%s", c.Frase)
	escrever("")
	escrever("Comparando")
	escrever("  antes:  %s contra %s, em %s", c.Antes.Cenario, c.Antes.Alvo, c.Antes.Inicio)
	escrever("  depois: %s contra %s, em %s", c.Depois.Cenario, c.Depois.Alvo, c.Depois.Inicio)
	escrever("")

	if c.Comparavel {
		escrever("A jornada inteira")
		escrever("  %s", c.Jornada.Frase)
		escrever("")

		escrever("Por passo")
		escrever("  %-26s %11s %11s %16s", "passo", "95% antes", "95% depois", "variacao")
		for _, passo := range c.Passos {
			nota := ""
			switch {
			case passo.Novo:
				nota = "  (passo novo)"
			case passo.Sumiu:
				nota = "  (nao existe mais)"
			}
			escrever("  %-26s %11s %11s %16s%s", cortar(passo.Passo, 26),
				milissegundos(passo.P95.Antes), milissegundos(passo.P95.Depois),
				variacao(passo.P95), nota)
		}
		escrever("")
		escrever("Erros")
		escrever("  %s", c.Erro.Frase)
		escrever("")
	}

	escrever("O que pode explicar a diferenca sem ser o servico")
	if len(c.Ressalvas) == 0 {
		escrever("  Nada: mesmo cenario, mesmo alvo, mesma maquina, mesmo plano de carga e mesma versao.")
	}
	for _, ressalva := range c.Ressalvas {
		escrever("  - %s", ressalva)
	}
	escrever("  Duas execucoes nao dao intervalo de confianca: variacao abaixo de %.0f%% e tratada como ruido.", comparacao.RuidoAceito*100)
	escrever("")
}

func variacao(diferenca comparacao.Diferenca) string {
	if diferenca.Sentido == comparacao.SentidoIgual {
		return "ruido"
	}
	magnitude := strings.Replace(comparacao.Magnitude(diferenca), " vezes", "x", 1)
	if diferenca.Sentido == comparacao.SentidoMelhor {
		return magnitude + " melhor"
	}
	return magnitude + " pior"
}
