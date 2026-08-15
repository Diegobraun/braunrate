package relatorio

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/Diegobraun/braunrate/metrica"
	"github.com/Diegobraun/braunrate/slo"
)

func LinhaDeProgresso(instantaneo metrica.Instantaneo, taxaAlvo float64, restante time.Duration) string {
	alerta := ""
	if instantaneo.Enviadas > 0 {
		proporcao := float64(instantaneo.DespachosAtrasados) / float64(instantaneo.Enviadas)
		if proporcao >= 0.01 {
			alerta = fmt.Sprintf("  ATENCAO: o gerador nao esta conseguindo manter a carga (%.1f%% em atraso)", proporcao*100)
		}
	}
	return fmt.Sprintf("carga %.0f/s | enviadas %d | concluidas %d | erros %d | metade em %.1f ms | 99%% em %.1f ms | faltam %s%s",
		taxaAlvo, instantaneo.Enviadas, instantaneo.Concluidas, instantaneo.Erros,
		instantaneo.LatenciaP50Ms, instantaneo.LatenciaP99Ms, restante.Round(time.Second), alerta)
}

// A saida tem duas camadas: a frase em portugues comum diz o que aconteceu, e o
// numero fica logo abaixo para quem precisa dele.
func Resumo(saida io.Writer, documento metrica.Documento, veredito slo.Veredito) {
	escrever := func(formato string, argumentos ...any) {
		fmt.Fprintf(saida, formato+"\n", argumentos...)
	}

	escrever("")
	escrever("%s — contra %s", documento.Execucao.Cenario, documento.Execucao.Alvo)
	escrever("")

	if len(veredito.Avaliacoes) > 0 {
		escrever("%s", veredito.Frase)
		escrever("")
	}

	global := documento.Global
	duracao := (time.Duration(documento.Execucao.DuracaoMs) * time.Millisecond).Round(100 * time.Millisecond)
	escrever("O que aconteceu")
	escrever("  %s requisicoes em %s, %.0f por segundo, %s de erro",
		milhar(global.Contagem), duracao, global.TaxaEfetiva, porcentagem(global.TaxaDeErro*100))
	escrever("  Metade das respostas em ate %s; 95%% em ate %s; 99%% em ate %s; a pior levou %s",
		milissegundos(global.Latencia.P50), milissegundos(global.Latencia.P95),
		milissegundos(global.Latencia.P99), milissegundos(global.Latencia.Maximo))
	escrever("")

	if documento.Jornada.Iniciadas > 0 {
		escrever("A jornada inteira")
		escrever("  %s", documento.Jornada.Frase)
		escrever("  metade %s | 95%% %s | 99%% %s | pior %s",
			milissegundos(documento.Jornada.Latencia.P50), milissegundos(documento.Jornada.Latencia.P95),
			milissegundos(documento.Jornada.Latencia.P99), milissegundos(documento.Jornada.Latencia.Maximo))
		escrever("")
	}

	escrever("Por passo")
	escrever("  %-26s %-3s %10s %9s %9s %9s %9s %9s %7s", "passo", "", "requisicoes", "metade", "95%", "99%", "99,9%", "pior", "erros")
	temPassoDeServico := false
	for _, passo := range documento.Passos {
		marca := "(1)"
		if passo.TipoDeLatencia == string(metrica.LatenciaDeServico) {
			marca = "(2)"
			temPassoDeServico = true
		}
		escrever("  %-26s %-3s %10s %9s %9s %9s %9s %9s %7d",
			cortar(passo.Nome, 26), marca, milhar(passo.Contagem),
			milissegundos(passo.Latencia.P50), milissegundos(passo.Latencia.P95),
			milissegundos(passo.Latencia.P99), milissegundos(passo.Latencia.P999),
			milissegundos(passo.Latencia.Maximo), passo.Erros)
	}
	escrever("")
	escrever("  (1) tempo contado do instante em que a requisicao deveria ter partido — inclui")
	escrever("      qualquer atraso e por isso nao esconde travada do alvo.")
	if temPassoDeServico {
		escrever("  (2) tempo de resposta puro, contado de quando o passo anterior terminou. Como")
		escrever("      esse passo depende do valor capturado antes dele, nao existe instante")
		escrever("      agendado proprio. Para a leitura honesta da jornada, use \"A jornada inteira\".")
	}
	escrever("")

	if len(veredito.Avaliacoes) > 0 {
		escrever("SLO")
		for _, avaliacao := range veredito.Avaliacoes {
			marca := "ok  "
			if !avaliacao.Passou {
				marca = "FALHA"
			}
			escrever("  %-5s %s", marca, avaliacao.Frase)
		}
		escrever("")
	}

	erros := errosPorClasse(documento)
	if len(erros) > 0 {
		escrever("Erros")
		for _, linha := range erros {
			escrever("  %-50s %s", linha.classe, milhar(linha.quantidade))
		}
		escrever("")
	}

	escrever("Confiabilidade da medicao")
	for _, aviso := range documento.Avisos {
		if aviso.Gravidade == metrica.GravidadeAlta {
			escrever("  RESULTADO INVALIDO: %s", aviso.Mensagem)
		} else {
			escrever("  Atencao: %s", aviso.Mensagem)
		}
		escrever("            %s", aviso.Evidencia)
	}
	if documento.Agendamento.DespachosAtrasados == 0 && documento.Agendamento.DescartadasPorLimiteDeVoo == 0 {
		escrever("  O gerador disparou todas as requisicoes na hora certa, entao os numeros acima valem.")
	}
	escrever("  Atraso tipico para disparar: %s; pior caso: %s (o tempo de resposta ja desconta isso)",
		milissegundos(documento.Agendamento.Desvio.P50), milissegundos(documento.Agendamento.Desvio.Maximo))
	escondido := documento.Global.Latencia.P99 - documento.Global.LatenciaDeServico.P99
	if escondido >= 1 {
		escrever("  Uma ferramenta de laco fechado teria reportado %s a menos no 99%%.", milissegundos(escondido))
	}
	escrever("")

	escrever("Ambiente")
	escrever("  %s %s/%s, %d nucleos | braunrate %s | %s",
		documento.Ambiente.Maquina, documento.Ambiente.SistemaOperacional, documento.Ambiente.Arquitetura,
		documento.Ambiente.Nucleos, documento.Versao, documento.Execucao.Inicio.Format("2006-01-02 15:04:05"))
	for _, variedade := range documento.Variedade {
		escrever("  %s", variedade.Frase)
	}
	if len(documento.Execucao.Sementes) > 0 {
		escrever("  Sementes dos dados: %s (mesma semente, mesmos dados)", sementes(documento.Execucao.Sementes))
	}
	if documento.Execucao.Autenticacoes > 0 {
		escrever("  Autenticacao obtida %d vez(es) e reaproveitada por todas as jornadas.", documento.Execucao.Autenticacoes)
		escrever("  Se o alvo tiver cache, rate limit ou sharding por token, este numero fica otimista.")
	}
	escrever("")
}

func sementes(valores map[string]int64) string {
	nomes := make([]string, 0, len(valores))
	for nome := range valores {
		nomes = append(nomes, nome)
	}
	sort.Strings(nomes)
	partes := make([]string, 0, len(nomes))
	for _, nome := range nomes {
		partes = append(partes, fmt.Sprintf("%s=%d", nome, valores[nome]))
	}
	return strings.Join(partes, ", ")
}

type linhaDeErro struct {
	classe     string
	quantidade int64
}

var nomeDeClasse = map[string]string{
	"rede":         "falha de rede",
	"timeout":      "tempo esgotado",
	"status":       "status HTTP inesperado",
	"assercao":     "conteudo fora do esperado",
	"correlacao":   "nao consegui capturar um valor",
	"configuracao": "erro de configuracao do cenario",
	"saturacao":    "gerador saturado",
	"graphql":      "erro no corpo da resposta GraphQL (com status 200)",
}

func errosPorClasse(documento metrica.Documento) []linhaDeErro {
	total := map[string]int64{}
	for _, passo := range documento.Passos {
		for classe, quantidade := range passo.ErrosPorClasse {
			nome := nomeDeClasse[classe]
			if nome == "" {
				nome = classe
			}
			total[nome] += quantidade
		}
	}
	linhas := make([]linhaDeErro, 0, len(total))
	for classe, quantidade := range total {
		linhas = append(linhas, linhaDeErro{classe: classe, quantidade: quantidade})
	}
	sort.Slice(linhas, func(i, j int) bool { return linhas[i].quantidade > linhas[j].quantidade })
	return linhas
}

func milissegundos(valor float64) string {
	switch {
	case valor >= 1000:
		return fmt.Sprintf("%.2f s", valor/1000)
	case valor >= 10:
		return fmt.Sprintf("%.0f ms", valor)
	case valor >= 1:
		return fmt.Sprintf("%.1f ms", valor)
	default:
		return fmt.Sprintf("%.3f ms", valor)
	}
}

func porcentagem(valor float64) string {
	if valor == 0 {
		return "0%"
	}
	if valor < 0.01 {
		return fmt.Sprintf("%.4f%%", valor)
	}
	return fmt.Sprintf("%.2f%%", valor)
}

func milhar(valor int64) string {
	texto := fmt.Sprintf("%d", valor)
	if len(texto) <= 3 {
		return texto
	}
	var partes []string
	for len(texto) > 3 {
		partes = append([]string{texto[len(texto)-3:]}, partes...)
		texto = texto[:len(texto)-3]
	}
	partes = append([]string{texto}, partes...)
	return strings.Join(partes, ".")
}

func cortar(texto string, tamanho int) string {
	if len(texto) <= tamanho {
		return texto
	}
	return strings.TrimSpace(texto[:tamanho-1]) + "…"
}
