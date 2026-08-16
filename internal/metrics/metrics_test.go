package metrics_test

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/Diegobraun/braunrate/internal/metrics"
	"github.com/Diegobraun/braunrate/internal/protocol"
)

var inicioFixo = time.Unix(1_700_000_000, 0).UTC()

func sample(scheduled time.Time, atrasoDeEnvio, service time.Duration, class protocol.ErrorClass) metrics.Sample {
	send := scheduled.Add(atrasoDeEnvio)
	return metrics.Sample{
		Step:        "consultar pedido",
		Key:         "GET /pedido",
		Protocol:    "http",
		ScheduledAt: scheduled,
		SentAt:      send,
		FinishedAt:  send.Add(service),
		Class:       class,
		Status:      200,
	}
}

func TestLatencyIsCountedFromScheduledInstant(t *testing.T) {
	a := sample(inicioFixo, 50*time.Millisecond, 10*time.Millisecond, protocol.Success)

	if obtained := a.CorrectedLatency(); obtained != 60*time.Millisecond {
		t.Errorf("latencia corrigida = %v, esperado 60ms", obtained)
	}
	if obtained := a.ServiceLatency(); obtained != 10*time.Millisecond {
		t.Errorf("latencia de servico = %v, esperado 10ms", obtained)
	}
}

func TestPercentileComesFromHistogramNotMean(t *testing.T) {
	aggregate := metrics.NewAggregate("passo", "chave", "http")
	for i := 0; i < 99; i++ {
		aggregate.Record(sample(inicioFixo, 0, 10*time.Millisecond, protocol.Success))
	}
	aggregate.Record(sample(inicioFixo, 0, 5*time.Second, protocol.Success))

	distribution := aggregate.Distribution()
	if math.Abs(distribution.P50-10) > 0.5 {
		t.Errorf("p50 = %.2f ms, esperado ~10", distribution.P50)
	}
	if math.Abs(distribution.Max-5000) > 5 {
		t.Errorf("max = %.2f ms, esperado ~5000", distribution.Max)
	}
	if distribution.Mean < 40 || distribution.Mean > 70 {
		t.Errorf("media = %.2f ms, esperado ~59; a media existe mas nao substitui percentil", distribution.Mean)
	}
	if distribution.P50 > distribution.Mean {
		t.Error("p50 acima da media com uma cauda longa indica calculo errado")
	}
}

func TestAggregatesMerge(t *testing.T) {
	first := metrics.NewAggregate("passo", "chave", "http")
	second := metrics.NewAggregate("passo", "chave", "http")

	for i := 0; i < 500; i++ {
		first.Record(sample(inicioFixo, 0, 10*time.Millisecond, protocol.Success))
	}
	for i := 0; i < 500; i++ {
		second.Record(sample(inicioFixo, 0, 100*time.Millisecond, protocol.ErrTimeout))
	}

	together := metrics.NewAggregate("passo", "chave", "http")
	together.Add(first)
	together.Add(second)

	if together.Count != 1000 {
		t.Errorf("contagem = %d, esperado 1000", together.Count)
	}
	if together.Errors() != 500 {
		t.Errorf("erros = %d, esperado 500", together.Errors())
	}
	distribution := together.Distribution()
	if math.Abs(distribution.P50-10) > 1 {
		t.Errorf("p50 do merge = %.2f ms, esperado ~10", distribution.P50)
	}
	if math.Abs(distribution.P99-100) > 2 {
		t.Errorf("p99 do merge = %.2f ms, esperado ~100", distribution.P99)
	}
}

func buildDocument(c *metrics.Collector, start, end time.Time) metrics.Document {
	c.Close()
	return metrics.BuildDocument(c, metrics.DocumentInput{
		Version: "teste", Scenario: "teste", Target: "http://alvo", Model: "aberto",
		Start: start, End: end, MaxInflight: 100,
	})
}

func TestBackPressureAboveOnePercentInvalidatesResult(t *testing.T) {
	collector := metrics.NewCollector(inicioFixo, 10*time.Millisecond)
	for i := 0; i < 100; i++ {
		scheduled := inicioFixo.Add(time.Duration(i) * 10 * time.Millisecond)
		delay := time.Duration(0)
		if i%20 == 0 {
			delay = 200 * time.Millisecond
		}
		collector.RecordDispatch(scheduled, scheduled.Add(delay), 100, 1)
		collector.Record(sample(scheduled, delay, 5*time.Millisecond, protocol.Success))
	}

	document := buildDocument(collector, inicioFixo, inicioFixo.Add(time.Second))

	if document.Valid() {
		t.Fatal("resultado com 5% de despachos atrasados nao pode ser dado como valido")
	}
	found := false
	for _, warning := range document.Warnings {
		if warning.Kind == "gerador_saturado" && warning.Severity == metrics.SeverityHigh {
			found = true
			if !strings.Contains(warning.Evidence, "%") {
				t.Errorf("evidencia sem proporcao: %q", warning.Evidence)
			}
		}
	}
	if !found {
		t.Fatalf("faltou aviso de gerador saturado: %+v", document.Warnings)
	}
}

func TestOccasionalDelayDoesNotInvalidateButIsReported(t *testing.T) {
	collector := metrics.NewCollector(inicioFixo, 10*time.Millisecond)
	for i := 0; i < 1000; i++ {
		scheduled := inicioFixo.Add(time.Duration(i) * time.Millisecond)
		delay := time.Duration(0)
		if i == 500 {
			delay = 50 * time.Millisecond
		}
		collector.RecordDispatch(scheduled, scheduled.Add(delay), 1000, 1)
		collector.Record(sample(scheduled, delay, 5*time.Millisecond, protocol.Success))
	}

	document := buildDocument(collector, inicioFixo, inicioFixo.Add(time.Second))

	if !document.Valid() {
		t.Fatalf("um atraso em mil nao deveria invalidar: %+v", document.Warnings)
	}
	found := false
	for _, warning := range document.Warnings {
		if warning.Kind == "gerador_com_atraso_pontual" {
			found = true
		}
	}
	if !found {
		t.Fatalf("o atraso pontual precisa aparecer no relatorio: %+v", document.Warnings)
	}
}

func TestLostSampleInvalidatesResult(t *testing.T) {
	collector := metrics.NewCollectorWithCapacity(inicioFixo, 10*time.Millisecond, 1)
	for i := 0; i < 200_000; i++ {
		collector.Record(sample(inicioFixo, 0, time.Millisecond, protocol.Success))
	}

	document := buildDocument(collector, inicioFixo, inicioFixo.Add(time.Second))

	if document.Scheduling.LostSamples == 0 {
		t.Fatal("com fila de capacidade 1 e 200 mil amostras o coletor precisa acusar perda")
	}
	if document.Valid() {
		t.Fatal("perda de amostra precisa invalidar o resultado")
	}
}

func TestInflightDropInvalidatesResult(t *testing.T) {
	collector := metrics.NewCollector(inicioFixo, 10*time.Millisecond)
	collector.RecordDispatch(inicioFixo, inicioFixo, 10, 1)
	collector.Record(sample(inicioFixo, 0, time.Millisecond, protocol.Success))
	collector.RecordInflightDrop()

	document := buildDocument(collector, inicioFixo, inicioFixo.Add(time.Second))

	if document.Valid() {
		t.Fatal("descarte por limite de voo precisa invalidar o resultado")
	}
}

func TestTimeSeriesUseEpochAlignedBuckets(t *testing.T) {
	collector := metrics.NewCollector(inicioFixo, 10*time.Millisecond)
	for i := 0; i < 5; i++ {
		scheduled := inicioFixo.Add(time.Duration(i) * 400 * time.Millisecond)
		collector.RecordDispatch(scheduled, scheduled, 2.5, 1)
		collector.Record(sample(scheduled, 0, 5*time.Millisecond, protocol.Success))
	}

	document := buildDocument(collector, inicioFixo, inicioFixo.Add(2*time.Second))

	for _, bucket := range document.Series {
		if bucket.StartEpochMs%1000 != 0 {
			t.Errorf("bucket nao alinhado ao epoch: %d", bucket.StartEpochMs)
		}
	}
	if len(document.Series) < 2 {
		t.Errorf("esperava mais de um bucket, obtido %d", len(document.Series))
	}
}
