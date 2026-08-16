package selfcheck

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/Diegobraun/braunrate/internal/engine"
	"github.com/Diegobraun/braunrate/internal/metrics"
	protocolohttp "github.com/Diegobraun/braunrate/internal/protocol/http"
	"github.com/Diegobraun/braunrate/internal/scenario"
	"github.com/Diegobraun/braunrate/internal/testsupport"
	"github.com/HdrHistogram/hdrhistogram-go"
)

const (
	latenciaDoAlvo         = 2 * time.Millisecond
	instanteDoCongelamento = time.Second
	duracaoDoCongelamento  = time.Second
	duracaoDaExecucao      = 3 * time.Second
	taxaDaExecucao         = 200.0
)

func subirAlvoQueCongela(t *testing.T) *testsupport.Servidor {
	t.Helper()
	servidor := testsupport.Novo(testsupport.Opcoes{
		Latencia:     latenciaDoAlvo,
		CongelarApos: instanteDoCongelamento,
		CongelarPor:  duracaoDoCongelamento,
	})
	if err := servidor.Iniciar("127.0.0.1:0"); err != nil {
		t.Fatalf("alvo nao subiu: %v", err)
	}
	t.Cleanup(func() { _ = servidor.Encerrar() })
	return servidor
}

func executarEmModeloAberto(t *testing.T, endereco string) metrics.Documento {
	t.Helper()
	c := scenario.Cenario{
		Nome: "auto-validacao de medicao",
		Alvo: endereco,
		Carga: scenario.PlanoDeCarga{
			Modelo: scenario.ChegadaAberta,
			Fases:  []scenario.Fase{{Tipo: scenario.FaseConstante, Ate: taxaDaExecucao, Durante: duracaoDaExecucao}},
		},
		Passos: []scenario.Passo{{
			Nome:         "consultar pedido",
			Protocolo:    "http",
			Configuracao: &protocolohttp.Configuracao{Metodo: http.MethodGet, Caminho: "/pedido"},
		}},
	}

	opcoes := engine.OpcoesPadrao()
	opcoes.Versao = "teste"
	m, err := engine.Novo(c, opcoes)
	if err != nil {
		t.Fatalf("motor nao subiu: %v", err)
	}
	return m.Executar(context.Background())
}

// O laco fechado so envia a proxima requisicao depois que a anterior responde;
// e assim que JMeter e Locust medem, e e por isso que a pausa some do p99 deles.
func executarEmLacoFechado(t *testing.T, endereco string) *hdrhistogram.Histogram {
	t.Helper()
	histograma := hdrhistogram.New(1, 600_000_000, 3)
	cliente := &http.Client{Timeout: 30 * time.Second}
	limite := time.Now().Add(duracaoDaExecucao)

	for time.Now().Before(limite) {
		inicio := time.Now()
		resposta, err := cliente.Get(endereco + "/pedido")
		if err != nil {
			continue
		}
		_, _ = io.Copy(io.Discard, resposta.Body)
		_ = resposta.Body.Close()
		_ = histograma.RecordValue(time.Since(inicio).Microseconds())
	}
	return histograma
}

func TestMedicaoRefleteCongelamentoDoAlvo(t *testing.T) {
	servidor := subirAlvoQueCongela(t)
	documento := executarEmModeloAberto(t, servidor.Endereco())

	pisoEsperado := float64(duracaoDoCongelamento.Milliseconds()) / 2
	if documento.Global.Latencia.P99 < pisoEsperado {
		t.Fatalf("p99 corrigida = %.1f ms; o alvo congelou por %s e a medicao precisa refletir isso (piso %.1f ms)",
			documento.Global.Latencia.P99, duracaoDoCongelamento, pisoEsperado)
	}
	if documento.Global.Latencia.Maximo < float64(duracaoDoCongelamento.Milliseconds())*0.9 {
		t.Errorf("maximo = %.1f ms, esperado proximo de %s", documento.Global.Latencia.Maximo, duracaoDoCongelamento)
	}
	if documento.Global.Contagem == 0 {
		t.Fatal("nenhuma requisicao concluida")
	}
	t.Logf("modelo aberto: p50 %.1f ms | p99 %.1f ms | max %.1f ms | n %d",
		documento.Global.Latencia.P50, documento.Global.Latencia.P99,
		documento.Global.Latencia.Maximo, documento.Global.Contagem)
}

func TestCongelamentoDoAlvoNaoEhConfundidoComSaturacaoDoGerador(t *testing.T) {
	servidor := subirAlvoQueCongela(t)
	documento := executarEmModeloAberto(t, servidor.Endereco())

	for _, aviso := range documento.Avisos {
		if aviso.Tipo == "gerador_saturado" {
			t.Fatalf("alvo congelado foi reportado como saturacao do gerador: %s | %s", aviso.Mensagem, aviso.Evidencia)
		}
	}

	encontrou := false
	for _, aviso := range documento.Avisos {
		if aviso.Tipo == "alvo_degradado" {
			encontrou = true
			t.Logf("aviso correto: %s | %s", aviso.Mensagem, aviso.Evidencia)
		}
	}
	if !encontrou {
		t.Fatalf("degradacao do alvo nao foi detectada; avisos: %+v", documento.Avisos)
	}
}

func TestLacoFechadoEsconderiaAPausaQueOModeloAbertoMostra(t *testing.T) {
	servidorAberto := subirAlvoQueCongela(t)
	documento := executarEmModeloAberto(t, servidorAberto.Endereco())

	servidorFechado := subirAlvoQueCongela(t)
	histograma := executarEmLacoFechado(t, servidorFechado.Endereco())

	p99Aberto := documento.Global.Latencia.P99
	p99Fechado := float64(histograma.ValueAtQuantile(99)) / 1000

	t.Logf("mesma pausa de %s no mesmo alvo:", duracaoDoCongelamento)
	t.Logf("  modelo aberto (braunrate): p99 %.1f ms sobre %d amostras", p99Aberto, documento.Global.Contagem)
	t.Logf("  laco fechado:              p99 %.1f ms sobre %d amostras", p99Fechado, histograma.TotalCount())
	t.Logf("  omissao coordenada: %.1f ms escondidos pelo laco fechado", p99Aberto-p99Fechado)

	if p99Fechado*5 > p99Aberto {
		t.Fatalf("o experimento nao demonstrou omissao coordenada: aberto %.1f ms, fechado %.1f ms",
			p99Aberto, p99Fechado)
	}
}
