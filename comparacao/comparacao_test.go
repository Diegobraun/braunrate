package comparacao_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Diegobraun/braunrate/comparacao"
	"github.com/Diegobraun/braunrate/metrica"
)

func documento(p95Jornada, p95Passo float64) metrica.Documento {
	inicio := time.Date(2026, 8, 15, 22, 0, 0, 0, time.UTC)
	return metrica.Documento{
		Ferramenta: "braunrate",
		Versao:     "0.3.0",
		Ambiente:   metrica.Ambiente{Maquina: "maquina-de-teste", Nucleos: 10},
		Execucao: metrica.Execucao{
			Cenario: "Jornada de cobranca", Alvo: "http://127.0.0.1:8080", Inicio: inicio,
			PlanoAplicado: []metrica.FaseAplicada{{Tipo: "patamar", Ate: 300, DuracaoMs: 10000}},
		},
		Jornada: metrica.Jornada{Iniciadas: 1500, Completas: 1500, Latencia: metrica.Distribuicao{P95: p95Jornada}},
		Passos: []metrica.ResultadoDePasso{
			{Nome: "consultar pedido", Contagem: 1500, Latencia: metrica.Distribuicao{P95: p95Passo, P99: p95Passo * 1.2}},
		},
		Global: metrica.ResultadoGlobal{Contagem: 1500, Latencia: metrica.Distribuicao{P95: p95Passo}},
	}
}

func TestRegressaoAparecePrimeiroEmPortuguesComum(t *testing.T) {
	c := comparacao.Comparar(documento(10, 5), documento(20, 10))
	if !strings.HasPrefix(c.Frase, "Ficou mais lento") {
		t.Errorf("a primeira frase precisa dizer o que aconteceu: %q", c.Frase)
	}
	if !strings.Contains(c.Frase, "de 10 ms para 20 ms") {
		t.Errorf("a frase precisa trazer os dois numeros: %q", c.Frase)
	}
	if c.Jornada.Sentido != comparacao.SentidoPiorou {
		t.Errorf("sentido veio %q", c.Jornada.Sentido)
	}
}

func TestMelhoraTambemEhDeclarada(t *testing.T) {
	c := comparacao.Comparar(documento(20, 10), documento(10, 5))
	if !strings.HasPrefix(c.Frase, "Ficou mais rapido") {
		t.Errorf("melhora precisa ser dita com a mesma clareza: %q", c.Frase)
	}
}

// Duas execucoes nao dao intervalo de confianca; chamar variacao pequena de
// regressao seria inventar precisao que a medicao nao tem.
func TestVariacaoPequenaEhTratadaComoRuido(t *testing.T) {
	c := comparacao.Comparar(documento(10, 5), documento(10.3, 5.1))
	if c.Jornada.Sentido != comparacao.SentidoIgual {
		t.Errorf("3%% de diferenca nao e regressao: %q", c.Jornada.Frase)
	}
	if !strings.Contains(c.Frase, "Sem mudanca que valha leitura") {
		t.Errorf("frase veio: %q", c.Frase)
	}
}

func TestAmbienteDiferenteViraRessalvaEhNaoConclusao(t *testing.T) {
	antes := documento(10, 5)
	depois := documento(20, 10)
	depois.Ambiente.Maquina = "outra-maquina"
	depois.Execucao.PlanoAplicado = []metrica.FaseAplicada{{Tipo: "patamar", Ate: 900, DuracaoMs: 10000}}

	c := comparacao.Comparar(antes, depois)
	juntas := strings.Join(c.Ressalvas, " | ")
	if !strings.Contains(juntas, "maquinas geradoras sao diferentes") {
		t.Errorf("maquina diferente precisa virar ressalva: %v", c.Ressalvas)
	}
	if !strings.Contains(juntas, "planos de carga sao diferentes") {
		t.Errorf("plano diferente precisa virar ressalva: %v", c.Ressalvas)
	}
	if !strings.Contains(c.Frase, "ressalva") {
		t.Errorf("a frase principal precisa avisar que existem ressalvas: %q", c.Frase)
	}
}

func TestResultadoInvalidoNaoEhComparado(t *testing.T) {
	antes := documento(10, 5)
	depois := documento(20, 10)
	depois.Avisos = []metrica.Aviso{{Gravidade: metrica.GravidadeAlta, Mensagem: "gerador saturado"}}

	c := comparacao.Comparar(antes, depois)
	if c.Comparavel {
		t.Error("execucao com gerador saturado nao serve de comparacao")
	}
	if !strings.Contains(c.Frase, "Nao da para comparar") {
		t.Errorf("frase veio: %q", c.Frase)
	}
}

func TestPassoNovoEhMarcadoEmVezDeVirarRegressao(t *testing.T) {
	antes := documento(10, 5)
	depois := documento(10, 5)
	depois.Passos = append(depois.Passos, metrica.ResultadoDePasso{
		Nome: "emitir recibo", Contagem: 1500, Latencia: metrica.Distribuicao{P95: 30},
	})

	c := comparacao.Comparar(antes, depois)
	var achou bool
	for _, passo := range c.Passos {
		if passo.Passo == "emitir recibo" {
			achou = passo.Novo
		}
	}
	if !achou {
		t.Error("passo que so existe na execucao nova precisa ser marcado como novo")
	}
}
