package relatorio

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/Diegobraun/braunrate/motor"
	"github.com/Diegobraun/braunrate/protocolo"
)

const limiteDeCorpo = 1200

// O modo de depuracao existe para que a correlacao quebrada apareca antes da
// carga, e nao depois de minutos de execucao.
func Depuracao(saida io.Writer, numero int, observacao motor.Observacao, mostrarCorpo bool) {
	escrever := func(formato string, argumentos ...any) {
		fmt.Fprintf(saida, formato+"\n", argumentos...)
	}

	marca := "ok"
	if observacao.Classe != protocolo.Sucesso {
		marca = "FALHOU"
	}
	escrever("")
	escrever("passo %d — %s   [%s em %s]", numero, observacao.Passo, marca, observacao.Duracao.Round(100_000))
	linhas := descreverConfiguracao(observacao.Configuracao)
	escrever("  requisicao: %s", linhas[0])
	for _, linha := range linhas[1:] {
		escrever("              %s", encurtar(linha))
	}

	if observacao.Resposta.Status > 0 {
		escrever("  resposta:   status %d, %d bytes", observacao.Resposta.Status, observacao.Resposta.Bytes)
	}
	if mostrarCorpo && len(observacao.Resposta.Corpo) > 0 {
		escrever("  corpo:      %s", encurtar(string(observacao.Resposta.Corpo)))
	}

	if len(observacao.Capturado) > 0 {
		escrever("  capturou:")
		for _, nome := range ordenar(observacao.Capturado) {
			escrever("    %s = %s", nome, encurtar(observacao.Capturado[nome]))
		}
	}

	for _, falha := range observacao.Falhas {
		escrever("  problema:   %s", falha)
	}
	if observacao.Classe != protocolo.Sucesso && len(observacao.Falhas) == 0 {
		escrever("  problema:   %s", nomeDeClasse[string(observacao.Classe)])
	}
}

func VariaveisDaIteracao(saida io.Writer, variaveis map[string]string) {
	fmt.Fprintln(saida, "")
	fmt.Fprintln(saida, "variaveis no fim da iteracao")
	for _, nome := range ordenar(variaveis) {
		fmt.Fprintf(saida, "  %s = %s\n", nome, encurtar(variaveis[nome]))
	}
}

func descreverConfiguracao(configuracao protocolo.Configuracao) []string {
	if configuracao == nil {
		return []string{"(nao montada)"}
	}
	if descritivel, sabe := configuracao.(protocolo.ConfiguracaoDescritivel); sabe {
		return descritivel.Descrever()
	}
	return []string{configuracao.ChaveDeAgregacao()}
}

func encurtar(texto string) string {
	texto = strings.TrimSpace(strings.ReplaceAll(texto, "\n", " "))
	if len(texto) > limiteDeCorpo {
		return texto[:limiteDeCorpo] + "…"
	}
	return texto
}

func ordenar(valores map[string]string) []string {
	nomes := make([]string, 0, len(valores))
	for nome := range valores {
		nomes = append(nomes, nome)
	}
	sort.Strings(nomes)
	return nomes
}
