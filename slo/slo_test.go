package slo_test

import (
	"strings"
	"testing"

	"github.com/Diegobraun/braunrate/cenario"
	"github.com/Diegobraun/braunrate/metrica"
	"github.com/Diegobraun/braunrate/slo"
)

func documentoDeExemplo() metrica.Documento {
	return metrica.Documento{
		Passos: []metrica.ResultadoDePasso{
			{Nome: "consultar pedido", Contagem: 1000, Erros: 0,
				Latencia: metrica.Distribuicao{P95: 210, P99: 300}},
			{Nome: "criar pedido", Contagem: 1000, Erros: 5,
				Latencia: metrica.Distribuicao{P95: 90, P99: 120}},
		},
		Global: metrica.ResultadoGlobal{Contagem: 2000, Erros: 5, TaxaDeErro: 0.0025,
			TaxaEfetiva: 800, Latencia: metrica.Distribuicao{P95: 150, P99: 260}},
	}
}

func regra(passo, metricaNome string, operador cenario.Operador, limite float64, unidade string) cenario.RegraDeSLO {
	return cenario.RegraDeSLO{Passo: passo, Global: passo == "global", Metrica: metricaNome,
		Operador: operador, Limite: limite, Unidade: unidade,
		Texto: metricaNome + " " + string(operador) + " limite"}
}

func TestSLOQuePassaEQueFalha(t *testing.T) {
	veredito := slo.Avaliar([]cenario.RegraDeSLO{
		regra("consultar pedido", "p95", cenario.OperadorMenor, 150, "ms"),
		regra("criar pedido", "p95", cenario.OperadorMenor, 150, "ms"),
	}, documentoDeExemplo())

	if veredito.Passou {
		t.Fatal("o veredito deveria falhar: consultar pedido teve p95 de 210 ms")
	}
	if len(veredito.Avaliacoes) != 2 {
		t.Fatalf("avaliacoes = %d", len(veredito.Avaliacoes))
	}
	if veredito.Avaliacoes[0].Passou || !veredito.Avaliacoes[1].Passou {
		t.Errorf("avaliacoes erradas: %+v", veredito.Avaliacoes)
	}
}

func TestFraseDeFalhaEhLegivelPorQuemNaoEhEngenheiro(t *testing.T) {
	veredito := slo.Avaliar([]cenario.RegraDeSLO{
		regra("consultar pedido", "p95", cenario.OperadorMenor, 150, "ms"),
	}, documentoDeExemplo())

	esperada := `Falhou: "consultar pedido" teve latencia p95 de 210 ms, acima do limite de 150 ms.`
	if veredito.Frase != esperada {
		t.Errorf("frase = %q\nesperada = %q", veredito.Frase, esperada)
	}
}

func TestTaxaDeErroEhAvaliadaEmPorcentagem(t *testing.T) {
	veredito := slo.Avaliar([]cenario.RegraDeSLO{
		regra("criar pedido", "erros", cenario.OperadorMenorOuIgual, 0, "%"),
	}, documentoDeExemplo())

	if veredito.Passou {
		t.Fatal("criar pedido teve 5 erros em 1000; a regra de 0% deveria falhar")
	}
	if !strings.Contains(veredito.Frase, "0.50%") {
		t.Errorf("a frase precisa dizer a taxa obtida: %q", veredito.Frase)
	}
}

func TestRegraGlobalUsaOsNumerosDoCenarioInteiro(t *testing.T) {
	veredito := slo.Avaliar([]cenario.RegraDeSLO{
		regra("global", "erros", cenario.OperadorMenor, 1, "%"),
		regra("global", "p99", cenario.OperadorMenor, 300, "ms"),
	}, documentoDeExemplo())

	if !veredito.Passou {
		t.Fatalf("as duas regras globais deveriam passar: %+v", veredito.Avaliacoes)
	}
	if !strings.Contains(veredito.Frase, "2 regras") {
		t.Errorf("frase = %q", veredito.Frase)
	}
}

func TestPassoInexistenteFalhaComMensagemClara(t *testing.T) {
	veredito := slo.Avaliar([]cenario.RegraDeSLO{
		regra("passo que nao existe", "p95", cenario.OperadorMenor, 100, "ms"),
	}, documentoDeExemplo())

	if veredito.Passou {
		t.Fatal("regra apontando para passo inexistente nao pode passar em silencio")
	}
	if !strings.Contains(veredito.Frase, "nao produziu nenhuma requisicao") {
		t.Errorf("frase = %q", veredito.Frase)
	}
}

func TestSemRegrasNaoHaVeredito(t *testing.T) {
	veredito := slo.Avaliar(nil, documentoDeExemplo())
	if !veredito.Passou {
		t.Error("sem regras declaradas nao ha o que falhar")
	}
	if !strings.Contains(veredito.Frase, "Sem SLO declarado") {
		t.Errorf("frase = %q", veredito.Frase)
	}
}
