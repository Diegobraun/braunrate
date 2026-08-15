package metrica_test

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/Diegobraun/braunrate/metrica"
	"github.com/Diegobraun/braunrate/protocolo"
)

var inicioFixo = time.Unix(1_700_000_000, 0).UTC()

func amostra(agendado time.Time, atrasoDeEnvio, servico time.Duration, classe protocolo.ClasseDeErro) metrica.Amostra {
	envio := agendado.Add(atrasoDeEnvio)
	return metrica.Amostra{
		Passo:             "consultar pedido",
		Chave:             "GET /pedido",
		Protocolo:         "http",
		InstanteAgendado:  agendado,
		InstanteDeEnvio:   envio,
		InstanteDeTermino: envio.Add(servico),
		Classe:            classe,
		Status:            200,
	}
}

func TestLatenciaEhContadaDoInstanteAgendado(t *testing.T) {
	a := amostra(inicioFixo, 50*time.Millisecond, 10*time.Millisecond, protocolo.Sucesso)

	if obtido := a.LatenciaCorrigida(); obtido != 60*time.Millisecond {
		t.Errorf("latencia corrigida = %v, esperado 60ms", obtido)
	}
	if obtido := a.LatenciaDeServico(); obtido != 10*time.Millisecond {
		t.Errorf("latencia de servico = %v, esperado 10ms", obtido)
	}
}

func TestPercentilVemDoHistogramaENaoDaMedia(t *testing.T) {
	agregado := metrica.NovoAgregado("passo", "chave", "http")
	for i := 0; i < 99; i++ {
		agregado.Registrar(amostra(inicioFixo, 0, 10*time.Millisecond, protocolo.Sucesso))
	}
	agregado.Registrar(amostra(inicioFixo, 0, 5*time.Second, protocolo.Sucesso))

	distribuicao := agregado.Distribuicao()
	if math.Abs(distribuicao.P50-10) > 0.5 {
		t.Errorf("p50 = %.2f ms, esperado ~10", distribuicao.P50)
	}
	if math.Abs(distribuicao.Maximo-5000) > 5 {
		t.Errorf("max = %.2f ms, esperado ~5000", distribuicao.Maximo)
	}
	if distribuicao.Media < 40 || distribuicao.Media > 70 {
		t.Errorf("media = %.2f ms, esperado ~59; a media existe mas nao substitui percentil", distribuicao.Media)
	}
	if distribuicao.P50 > distribuicao.Media {
		t.Error("p50 acima da media com uma cauda longa indica calculo errado")
	}
}

func TestAgregadosSaoMergeaveis(t *testing.T) {
	primeiro := metrica.NovoAgregado("passo", "chave", "http")
	segundo := metrica.NovoAgregado("passo", "chave", "http")

	for i := 0; i < 500; i++ {
		primeiro.Registrar(amostra(inicioFixo, 0, 10*time.Millisecond, protocolo.Sucesso))
	}
	for i := 0; i < 500; i++ {
		segundo.Registrar(amostra(inicioFixo, 0, 100*time.Millisecond, protocolo.ErroDeTimeout))
	}

	juntos := metrica.NovoAgregado("passo", "chave", "http")
	juntos.Somar(primeiro)
	juntos.Somar(segundo)

	if juntos.Contagem != 1000 {
		t.Errorf("contagem = %d, esperado 1000", juntos.Contagem)
	}
	if juntos.Erros() != 500 {
		t.Errorf("erros = %d, esperado 500", juntos.Erros())
	}
	distribuicao := juntos.Distribuicao()
	if math.Abs(distribuicao.P50-10) > 1 {
		t.Errorf("p50 do merge = %.2f ms, esperado ~10", distribuicao.P50)
	}
	if math.Abs(distribuicao.P99-100) > 2 {
		t.Errorf("p99 do merge = %.2f ms, esperado ~100", distribuicao.P99)
	}
}

func montarDocumento(c *metrica.Coletor, inicio, fim time.Time) metrica.Documento {
	c.Encerrar()
	return metrica.MontarDocumento(c, metrica.EntradaDoDocumento{
		Versao: "teste", Cenario: "teste", Alvo: "http://alvo", Modelo: "aberto",
		Inicio: inicio, Fim: fim, LimiteDeVoo: 100,
	})
}

func TestBackPressureAcimaDeUmPorCentoInvalidaOResultado(t *testing.T) {
	coletor := metrica.NovoColetor(inicioFixo, 10*time.Millisecond)
	for i := 0; i < 100; i++ {
		agendado := inicioFixo.Add(time.Duration(i) * 10 * time.Millisecond)
		atraso := time.Duration(0)
		if i%20 == 0 {
			atraso = 200 * time.Millisecond
		}
		coletor.RegistrarDespacho(agendado, agendado.Add(atraso), 100, 1)
		coletor.Registrar(amostra(agendado, atraso, 5*time.Millisecond, protocolo.Sucesso))
	}

	documento := montarDocumento(coletor, inicioFixo, inicioFixo.Add(time.Second))

	if documento.ResultadoValido() {
		t.Fatal("resultado com 5% de despachos atrasados nao pode ser dado como valido")
	}
	encontrou := false
	for _, aviso := range documento.Avisos {
		if aviso.Tipo == "gerador_saturado" && aviso.Gravidade == metrica.GravidadeAlta {
			encontrou = true
			if !strings.Contains(aviso.Evidencia, "%") {
				t.Errorf("evidencia sem proporcao: %q", aviso.Evidencia)
			}
		}
	}
	if !encontrou {
		t.Fatalf("faltou aviso de gerador saturado: %+v", documento.Avisos)
	}
}

func TestAtrasoPontualNaoInvalidaMasApareceNoRelatorio(t *testing.T) {
	coletor := metrica.NovoColetor(inicioFixo, 10*time.Millisecond)
	for i := 0; i < 1000; i++ {
		agendado := inicioFixo.Add(time.Duration(i) * time.Millisecond)
		atraso := time.Duration(0)
		if i == 500 {
			atraso = 50 * time.Millisecond
		}
		coletor.RegistrarDespacho(agendado, agendado.Add(atraso), 1000, 1)
		coletor.Registrar(amostra(agendado, atraso, 5*time.Millisecond, protocolo.Sucesso))
	}

	documento := montarDocumento(coletor, inicioFixo, inicioFixo.Add(time.Second))

	if !documento.ResultadoValido() {
		t.Fatalf("um atraso em mil nao deveria invalidar: %+v", documento.Avisos)
	}
	encontrou := false
	for _, aviso := range documento.Avisos {
		if aviso.Tipo == "gerador_com_atraso_pontual" {
			encontrou = true
		}
	}
	if !encontrou {
		t.Fatalf("o atraso pontual precisa aparecer no relatorio: %+v", documento.Avisos)
	}
}

func TestAmostraPerdidaInvalidaOResultado(t *testing.T) {
	coletor := metrica.NovoColetorComCapacidade(inicioFixo, 10*time.Millisecond, 1)
	for i := 0; i < 200_000; i++ {
		coletor.Registrar(amostra(inicioFixo, 0, time.Millisecond, protocolo.Sucesso))
	}

	documento := montarDocumento(coletor, inicioFixo, inicioFixo.Add(time.Second))

	if documento.Agendamento.AmostrasPerdidas == 0 {
		t.Fatal("com fila de capacidade 1 e 200 mil amostras o coletor precisa acusar perda")
	}
	if documento.ResultadoValido() {
		t.Fatal("perda de amostra precisa invalidar o resultado")
	}
}

func TestDescartePorLimiteDeVooInvalidaOResultado(t *testing.T) {
	coletor := metrica.NovoColetor(inicioFixo, 10*time.Millisecond)
	coletor.RegistrarDespacho(inicioFixo, inicioFixo, 10, 1)
	coletor.Registrar(amostra(inicioFixo, 0, time.Millisecond, protocolo.Sucesso))
	coletor.RegistrarDescartePorLimiteDeVoo()

	documento := montarDocumento(coletor, inicioFixo, inicioFixo.Add(time.Second))

	if documento.ResultadoValido() {
		t.Fatal("descarte por limite de voo precisa invalidar o resultado")
	}
}

func TestSeriesTemporaisUsamBucketAlinhadoAoEpoch(t *testing.T) {
	coletor := metrica.NovoColetor(inicioFixo, 10*time.Millisecond)
	for i := 0; i < 5; i++ {
		agendado := inicioFixo.Add(time.Duration(i) * 400 * time.Millisecond)
		coletor.RegistrarDespacho(agendado, agendado, 2.5, 1)
		coletor.Registrar(amostra(agendado, 0, 5*time.Millisecond, protocolo.Sucesso))
	}

	documento := montarDocumento(coletor, inicioFixo, inicioFixo.Add(2*time.Second))

	for _, bucket := range documento.Series {
		if bucket.InicioEpochMs%1000 != 0 {
			t.Errorf("bucket nao alinhado ao epoch: %d", bucket.InicioEpochMs)
		}
	}
	if len(documento.Series) < 2 {
		t.Errorf("esperava mais de um bucket, obtido %d", len(documento.Series))
	}
}
