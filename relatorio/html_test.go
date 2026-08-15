package relatorio_test

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Diegobraun/braunrate/metrica"
	"github.com/Diegobraun/braunrate/relatorio"
)

func documentoDeExemplo() metrica.Documento {
	inicio := time.Date(2026, 8, 15, 22, 0, 0, 0, time.UTC)
	return metrica.Documento{
		VersaoDoFormato: metrica.VersaoDoFormatoDeResultado,
		Ferramenta:      "braunrate",
		Versao:          "0.3.0",
		Ambiente:        metrica.Ambiente{Maquina: "maquina-de-teste", SistemaOperacional: "darwin", Arquitetura: "arm64", Nucleos: 10},
		Execucao: metrica.Execucao{
			Cenario: "Jornada de cobranca", Alvo: "http://127.0.0.1:8080",
			Inicio: inicio, Fim: inicio.Add(10 * time.Second), DuracaoMs: 10000,
			Modelo: "aberto", MaximoSimultaneas: 20000, Autenticacoes: 1,
			PlanoAplicado: []metrica.FaseAplicada{{Tipo: "patamar", Ate: 300, DuracaoMs: 10000}},
		},
		Agendamento: metrica.Agendamento{Enviadas: 3000, Concluidas: 3000, Desvio: metrica.Distribuicao{P50: 0.01, Maximo: 1.2}},
		Jornada: metrica.Jornada{
			Iniciadas: 1500, Completas: 1500,
			Latencia: metrica.Distribuicao{P50: 8.7, P95: 9.5, P99: 10, Maximo: 18},
			Frase:    "Todas as 1500 jornadas chegaram ao fim; metade levou ate 9 ms e 95% ate 10 ms, contados do instante em que deveriam ter comecado.",
		},
		Passos: []metrica.ResultadoDePasso{
			{Nome: "consultar pedido", TipoDeLatencia: string(metrica.LatenciaCorrigida), Contagem: 1500,
				Latencia: metrica.Distribuicao{P50: 4.3, P95: 4.9, P99: 5.3, P999: 6.2, Maximo: 13}},
			{Nome: "pagar fatura", TipoDeLatencia: string(metrica.LatenciaDeServico), Contagem: 1500, Erros: 3,
				ErrosPorClasse: map[string]int64{"status": 3},
				Latencia:       metrica.Distribuicao{P50: 4.3, P95: 4.8, P99: 5.1, P999: 5.8, Maximo: 11}},
		},
		Global: metrica.ResultadoGlobal{
			Contagem: 3000, Sucessos: 2997, Erros: 3, TaxaDeErro: 0.001, TaxaEfetiva: 300,
			Latencia:          metrica.Distribuicao{P50: 4.3, P95: 4.9, P99: 9.8},
			LatenciaDeServico: metrica.Distribuicao{P50: 4.3, P95: 4.8, P99: 5.1},
		},
		Series: []metrica.Bucket{
			{InicioEpochMs: 1000, Enviadas: 300, Concluidas: 300, LatenciaP50Ms: 4.2, LatenciaP99Ms: 5.1},
			{InicioEpochMs: 2000, Enviadas: 300, Concluidas: 300, LatenciaP50Ms: 4.3, LatenciaP99Ms: 5.4},
			{InicioEpochMs: 3000, Enviadas: 300, Concluidas: 299, Erros: 1, LatenciaP50Ms: 4.4, LatenciaP99Ms: 9.9},
		},
	}
}

func gerar(t *testing.T, documento metrica.Documento) string {
	t.Helper()
	var saida strings.Builder
	if err := relatorio.HTML(&saida, documento); err != nil {
		t.Fatalf("nao gerou o HTML: %v", err)
	}
	return saida.String()
}

func TestOTopoDoRelatorioEhUmaFraseEhNaoUmaTabela(t *testing.T) {
	documento := documentoDeExemplo()
	documento.SLO = metrica.Veredito{Passou: true, Frase: "Passou: as 3 regras de SLO foram atendidas."}
	pagina := gerar(t, documento)

	titulo := regexp.MustCompile(`(?s)<h1[^>]*>(.*?)</h1>`).FindStringSubmatch(pagina)
	if titulo == nil {
		t.Fatal("o relatorio nao tem titulo")
	}
	if titulo[1] != "Passou: as 3 regras de SLO foram atendidas." {
		t.Errorf("o topo precisa ser a frase do veredito, veio: %q", titulo[1])
	}
	if indice := strings.Index(pagina, "<table"); indice < strings.Index(pagina, "</h1>") {
		t.Error("existe tabela antes da frase de veredito")
	}
}

func TestRelatorioDeFalhaMostraOMotivoNoTopo(t *testing.T) {
	documento := documentoDeExemplo()
	documento.SLO = metrica.Veredito{
		Passou: false,
		Frase:  `Falhou: "pagar fatura" teve latencia p95 de 210 ms, acima do limite de 150 ms.`,
		Avaliacoes: []metrica.Avaliacao{
			{Passo: "pagar fatura", Passou: false, Frase: `Falhou: "pagar fatura" teve latencia p95 de 210 ms, acima do limite de 150 ms.`},
		},
	}
	pagina := gerar(t, documento)

	if !strings.Contains(pagina, `<h1 class="falhou">`) {
		t.Error("falha de SLO precisa aparecer como falha no topo")
	}
	if !strings.Contains(pagina, "acima do limite de 150 ms") {
		t.Error("o motivo da falha nao aparece")
	}
}

func TestResultadoInvalidoNaoEhApresentadoComoNumeroDoAlvo(t *testing.T) {
	documento := documentoDeExemplo()
	documento.SLO = metrica.Veredito{Passou: true, Frase: "Passou: as 3 regras de SLO foram atendidas."}
	documento.Avisos = []metrica.Aviso{{
		Tipo: "gerador_saturado", Gravidade: metrica.GravidadeAlta,
		Mensagem:  "o gerador nao sustentou a taxa alvo",
		Evidencia: "12% dos despachos atrasaram",
	}}
	pagina := gerar(t, documento)

	if strings.Contains(pagina, `<h1 class="passou">`) {
		t.Error("com o gerador saturado o topo nao pode dizer que passou")
	}
	if !strings.Contains(pagina, "Resultado invalido") {
		t.Error("o topo precisa declarar que o resultado nao vale")
	}
	if !strings.Contains(pagina, "medem o gerador, nao o alvo") {
		t.Error("falta a leitura em portugues comum do resultado invalido")
	}
}

func TestRelatorioDistingueLatenciaCorrigidaDeLatenciaDeServico(t *testing.T) {
	pagina := gerar(t, documentoDeExemplo())
	if !strings.Contains(pagina, "(1)") || !strings.Contains(pagina, "(2)") {
		t.Error("os dois tipos de latencia precisam estar marcados por passo")
	}
	if !strings.Contains(pagina, "nao tem instante agendado proprio") {
		t.Error("falta a explicacao do que e latencia de servico")
	}
	if !strings.Contains(pagina, "A jornada inteira") {
		t.Error("falta a metrica que continua honesta para a jornada toda")
	}
}

func TestRelatorioDeclaraALimitacaoDeTokenUnico(t *testing.T) {
	pagina := gerar(t, documentoDeExemplo())
	if !strings.Contains(pagina, "cache, rate limit ou sharding por token") {
		t.Error("execucao com autenticacao precisa declarar a limitacao de token unico")
	}
}

// Relatorio de carga costuma ser aberto de dentro de rede fechada ou anexado
// em ticket; se depender de rede, abre quebrado justamente onde importa.
func TestRelatorioNaoBuscaNadaNaRede(t *testing.T) {
	pagina := gerar(t, documentoDeExemplo())
	proibidos := []string{"<script", "src=", "@import", "cdn.", "https://fonts", "<link"}
	for _, proibido := range proibidos {
		if strings.Contains(pagina, proibido) {
			t.Errorf("o relatorio deixou de ser autocontido: encontrei %q", proibido)
		}
	}
}

func TestRelatorioSemSLODizQueNaoAprovaNemReprova(t *testing.T) {
	pagina := gerar(t, documentoDeExemplo())
	if !strings.Contains(pagina, "nao aprova nem reprova") {
		t.Error("sem slo declarado o relatorio precisa dizer que nao decide nada")
	}
}

func TestSerieTemporalViraGraficoSemBiblioteca(t *testing.T) {
	pagina := gerar(t, documentoDeExemplo())
	if !strings.Contains(pagina, "<svg") || !strings.Contains(pagina, "<polyline") {
		t.Error("a serie temporal nao virou grafico")
	}
}

func TestCSVSeparaLatenciaCorrigidaDeLatenciaDeServico(t *testing.T) {
	var saida strings.Builder
	if err := relatorio.CSV(&saida, documentoDeExemplo()); err != nil {
		t.Fatalf("nao gerou o CSV: %v", err)
	}
	linhas := strings.Split(strings.TrimSpace(saida.String()), "\n")
	if len(linhas) != 5 {
		t.Fatalf("esperava cabecalho, jornada, dois passos e global; vieram %d linhas", len(linhas))
	}
	if !strings.Contains(linhas[0], "tipo_de_latencia") {
		t.Error("o CSV precisa dizer de que tipo e cada latencia")
	}
	if !strings.HasPrefix(linhas[1], "Jornada de cobranca,http://127.0.0.1:8080") || !strings.Contains(linhas[1], "jornada inteira") {
		t.Errorf("a primeira linha de dados precisa ser a jornada: %s", linhas[1])
	}
	if !strings.Contains(linhas[3], ",servico,") {
		t.Errorf("o passo de latencia de servico precisa estar marcado: %s", linhas[3])
	}
}
