package relatorio

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/Diegobraun/braunrate/metrica"
)

func LinhaDeProgresso(instantaneo metrica.Instantaneo, taxaAlvo float64, restante time.Duration) string {
	aviso := ""
	if instantaneo.Enviadas > 0 {
		proporcao := float64(instantaneo.DespachosAtrasados) / float64(instantaneo.Enviadas)
		if proporcao >= 0.01 {
			aviso = fmt.Sprintf("  BACK-PRESSURE: %.1f%% dos despachos atrasados (gerador saturado)", proporcao*100)
		}
	}
	return fmt.Sprintf("alvo %.0f/s | enviadas %d | concluidas %d | erros %d | p50 %.1f ms | p99 %.1f ms | restam %s%s",
		taxaAlvo, instantaneo.Enviadas, instantaneo.Concluidas, instantaneo.Erros,
		instantaneo.LatenciaP50Ms, instantaneo.LatenciaP99Ms, restante.Round(time.Second), aviso)
}

func Resumo(saida io.Writer, documento metrica.Documento) {
	escrever := func(formato string, argumentos ...any) {
		fmt.Fprintf(saida, formato+"\n", argumentos...)
	}

	escrever("")
	escrever("cenario   %s", documento.Execucao.Cenario)
	escrever("alvo      %s", documento.Execucao.Alvo)
	escrever("modelo    chegada %s | duracao %s | limite de voo %d",
		documento.Execucao.Modelo,
		(time.Duration(documento.Execucao.DuracaoMs) * time.Millisecond).Round(100*time.Millisecond),
		documento.Execucao.LimiteDeVoo)
	escrever("ambiente  %s %s/%s, %d nucleos, %s",
		documento.Ambiente.Maquina, documento.Ambiente.SistemaOperacional,
		documento.Ambiente.Arquitetura, documento.Ambiente.Nucleos, documento.Ambiente.VersaoDoGo)
	escrever("")

	for _, aviso := range documento.Avisos {
		marca := "aviso"
		if aviso.Gravidade == metrica.GravidadeAlta {
			marca = "RESULTADO INVALIDO"
		}
		escrever("[%s] %s", marca, aviso.Mensagem)
		escrever("           %s", aviso.Evidencia)
	}
	if len(documento.Avisos) > 0 {
		escrever("")
	}

	escrever("agendamento")
	escrever("  enviadas %d | concluidas %d | descartadas por limite de voo %d | pico em voo %d",
		documento.Agendamento.Enviadas, documento.Agendamento.Concluidas,
		documento.Agendamento.DescartadasPorLimiteDeVoo, documento.Agendamento.PicoEmVoo)
	escrever("  desvio de agendamento  p50 %.3f ms | p99 %.3f ms | max %.3f ms | atrasados %d",
		documento.Agendamento.Desvio.P50, documento.Agendamento.Desvio.P99,
		documento.Agendamento.Desvio.Maximo, documento.Agendamento.DespachosAtrasados)
	escrever("")

	escrever("latencia por passo (contada do instante agendado)")
	escrever("  %-28s %8s %8s %8s %8s %8s %8s %8s %8s", "passo", "n", "p50", "p90", "p95", "p99", "p99.9", "max", "erros")
	for _, passo := range documento.Passos {
		escrever("  %-28s %8d %8.1f %8.1f %8.1f %8.1f %8.1f %8.1f %8d",
			cortar(passo.Nome, 28), passo.Contagem, passo.Latencia.P50, passo.Latencia.P90,
			passo.Latencia.P95, passo.Latencia.P99, passo.Latencia.P999, passo.Latencia.Maximo, passo.Erros)
	}
	escrever("")

	global := documento.Global
	escrever("global")
	escrever("  requisicoes %d | taxa efetiva %.1f/s | erros %d (%.3f%%)",
		global.Contagem, global.TaxaEfetiva, global.Erros, global.TaxaDeErro*100)
	escrever("  latencia corrigida  p50 %.1f | p95 %.1f | p99 %.1f | p99.9 %.1f | max %.1f (ms)",
		global.Latencia.P50, global.Latencia.P95, global.Latencia.P99, global.Latencia.P999, global.Latencia.Maximo)
	escrever("  latencia de servico p50 %.1f | p95 %.1f | p99 %.1f | p99.9 %.1f | max %.1f (ms)",
		global.LatenciaDeServico.P50, global.LatenciaDeServico.P95, global.LatenciaDeServico.P99,
		global.LatenciaDeServico.P999, global.LatenciaDeServico.Maximo)
	escrever("  omissao coordenada  p99 corrigida menos p99 de servico = %.1f ms",
		global.Latencia.P99-global.LatenciaDeServico.P99)

	erros := errosPorClasse(documento)
	if len(erros) > 0 {
		escrever("")
		escrever("erros por classe")
		for _, linha := range erros {
			escrever("  %-24s %d", linha.classe, linha.quantidade)
		}
	}
	escrever("")
}

type linhaDeErro struct {
	classe     string
	quantidade int64
}

func errosPorClasse(documento metrica.Documento) []linhaDeErro {
	total := map[string]int64{}
	for _, passo := range documento.Passos {
		for classe, quantidade := range passo.ErrosPorClasse {
			total[classe] += quantidade
		}
	}
	linhas := make([]linhaDeErro, 0, len(total))
	for classe, quantidade := range total {
		linhas = append(linhas, linhaDeErro{classe: classe, quantidade: quantidade})
	}
	sort.Slice(linhas, func(i, j int) bool { return linhas[i].quantidade > linhas[j].quantidade })
	return linhas
}

func cortar(texto string, tamanho int) string {
	if len(texto) <= tamanho {
		return texto
	}
	return strings.TrimSpace(texto[:tamanho-1]) + "…"
}
