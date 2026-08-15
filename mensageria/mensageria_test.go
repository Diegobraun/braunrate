package mensageria_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Diegobraun/braunrate/alvo"
	"github.com/Diegobraun/braunrate/cenario"
	"github.com/Diegobraun/braunrate/metrica"
	"github.com/Diegobraun/braunrate/motor"
	"github.com/Diegobraun/braunrate/protocolo"
	_ "github.com/Diegobraun/braunrate/protocolo/aguardar"
	_ "github.com/Diegobraun/braunrate/protocolo/amqp"
	_ "github.com/Diegobraun/braunrate/protocolo/kafka"
	"github.com/Diegobraun/braunrate/slo"
	"github.com/segmentio/kafka-go"
)

// Mensageria so pode ser medida contra um broker de verdade: um dublê responde
// no tempo que o teste mandar, e o numero deixaria de significar qualquer coisa.
// Sem broker o teste declara que pulou, em vez de fingir que passou.
func brokerKafka(t *testing.T) string {
	t.Helper()
	endereco := os.Getenv("BRAUNRATE_KAFKA")
	if endereco == "" {
		t.Skip("sem BRAUNRATE_KAFKA: teste de mensageria pulado, nao aprovado")
	}
	return endereco
}

func brokerAMQP(t *testing.T) string {
	t.Helper()
	endereco := os.Getenv("BRAUNRATE_AMQP")
	if endereco == "" {
		t.Skip("sem BRAUNRATE_AMQP: teste de mensageria pulado, nao aprovado")
	}
	return endereco
}

func topico(t *testing.T, brokers string, nome string, particoes int) string {
	t.Helper()
	completo := fmt.Sprintf("%s-%d", nome, time.Now().UnixNano())
	conexao, err := kafka.Dial("tcp", brokers)
	if err != nil {
		t.Fatalf("nao consegui falar com o broker: %v", err)
	}
	defer conexao.Close()
	if err := conexao.CreateTopics(kafka.TopicConfig{
		Topic:             completo,
		NumPartitions:     particoes,
		ReplicationFactor: 1,
	}); err != nil {
		t.Fatalf("nao consegui criar o topico: %v", err)
	}
	return completo
}

func executar(t *testing.T, conteudo string) (metrica.Documento, slo.Veredito) {
	t.Helper()
	raiz := t.TempDir()
	caminho := filepath.Join(raiz, "cenario.yaml")
	if err := os.WriteFile(caminho, []byte(conteudo), 0o644); err != nil {
		t.Fatalf("nao consegui escrever o cenario: %v", err)
	}
	c, err := cenario.CarregarArquivo(caminho)
	if err != nil {
		t.Fatalf("cenario nao carregou: %v", err)
	}
	if err := c.Validar(); err != nil {
		t.Fatalf("cenario invalido: %v", err)
	}
	opcoes := motor.OpcoesPadrao()
	opcoes.RaizDeDados = raiz
	m, err := motor.Novo(c, opcoes)
	if err != nil {
		t.Fatalf("motor nao subiu: %v", err)
	}
	documento := m.Executar(context.Background())
	t.Cleanup(func() { protocolo.EncerrarTodos() })
	return documento, slo.Avaliar(c.SLO, documento)
}

const cenarioDeCadeia = `
nome: Cadeia assincrona
alvo: %s

dados:
  pedidos:
    gerar: { id: uuid }

carga:
  perfis:
    - constante: { taxa: 30/s, durante: 2s }

cenario:
  - kafka:
      topico: %s
      chave: "%s"
      valor: { pedido: "${pedidos.id}" }

  - aguardar:
      kafka: { topico: %s }
      chave: "${pedidos.id}"
      timeout: 15s

slo:
  - global: { erros: < 1 }
`

// A medicao da cadeia assincrona: o passo aguardar so termina quando a mensagem
// daquela iteracao aparece do outro lado, entao a jornada mede produtor,
// processador e consumidor juntos — que e o que o usuario final sente.
func TestCadeiaAssincronaMedeDoProdutorAoConsumidor(t *testing.T) {
	brokers := brokerKafka(t)
	entrada := topico(t, brokers, "entrada", 4)
	saida := topico(t, brokers, "saida", 4)

	const atrasoDoProcessador = 40 * time.Millisecond
	processador := alvo.NovoProcessador(alvo.OpcoesDeProcessador{
		Brokers: strings.Split(brokers, ","),
		Entrada: entrada,
		Saida:   saida,
		Grupo:   "processador-" + saida,
		Atraso:  atrasoDoProcessador,
	})
	if err := processador.Iniciar(); err != nil {
		t.Fatalf("processador nao subiu: %v", err)
	}
	t.Cleanup(func() { _ = processador.Encerrar() })
	time.Sleep(3 * time.Second)

	documento, veredito := executar(t, fmt.Sprintf(cenarioDeCadeia,
		brokers, entrada, "${pedidos.id}", saida))

	if !veredito.Passou {
		t.Fatalf("a cadeia deveria fechar sem erro: %s", veredito.Frase)
	}
	if documento.Jornada.Completas == 0 {
		t.Fatal("nenhuma jornada chegou ao fim: a mensagem produzida nunca voltou")
	}

	porNome := map[string]metrica.ResultadoDePasso{}
	for _, passo := range documento.Passos {
		porNome[passo.Nome] = passo
	}
	producao, temProducao := porNome["kafka produzir "+entrada]
	espera, temEspera := porNome["aguardar "+saida]
	if !temProducao || !temEspera {
		t.Fatalf("o relatorio precisa de uma linha por destino: %+v", porNome)
	}
	if espera.Latencia.P50 < float64(atrasoDoProcessador.Milliseconds()) {
		t.Errorf("a espera mediu %0.1f ms, menos que os %s do processador: nao mediu a cadeia",
			espera.Latencia.P50, atrasoDoProcessador)
	}
	if producao.Latencia.P50 > espera.Latencia.P50 {
		t.Errorf("produzir (%0.1f ms) nao deveria custar mais que a cadeia inteira (%0.1f ms)",
			producao.Latencia.P50, espera.Latencia.P50)
	}
	if documento.Jornada.Latencia.P50 < espera.Latencia.P50 {
		t.Error("a jornada precisa incluir producao e espera")
	}
}

// Chave de particao sempre igual manda tudo para a mesma particao: o resto do
// cluster fica parado e o numero fica otimista, exatamente como o assinante
// unico do bug de identidade.
func TestChaveDeParticaoFixaInvalidaOResultado(t *testing.T) {
	brokers := brokerKafka(t)
	entrada := topico(t, brokers, "fixa", 4)
	saida := topico(t, brokers, "fixa-saida", 4)

	processador := alvo.NovoProcessador(alvo.OpcoesDeProcessador{
		Brokers: strings.Split(brokers, ","),
		Entrada: entrada,
		Saida:   saida,
		Grupo:   "processador-" + saida,
	})
	if err := processador.Iniciar(); err != nil {
		t.Fatalf("processador nao subiu: %v", err)
	}
	t.Cleanup(func() { _ = processador.Encerrar() })
	time.Sleep(3 * time.Second)

	documento, _ := executar(t, fmt.Sprintf(cenarioDeCadeia,
		brokers, entrada, "sempre-a-mesma", saida))

	var achou bool
	for _, aviso := range documento.Avisos {
		if aviso.Tipo != "variedade_ausente" || !strings.Contains(aviso.Mensagem, "particao") {
			continue
		}
		achou = true
		if aviso.Gravidade != metrica.GravidadeAlta {
			t.Errorf("gravidade = %q, esperava alta", aviso.Gravidade)
		}
		if !strings.Contains(aviso.Mensagem, "chave da mensagem variar") {
			t.Errorf("a mensagem precisa dizer o que fazer: %q", aviso.Mensagem)
		}
	}
	if !achou {
		t.Fatalf("carga inteira numa particao so precisa avisar; avisos: %+v", documento.Avisos)
	}
	if documento.ResultadoValido() {
		t.Error("resultado concentrado numa particao nao pode ser dado como valido")
	}
}

const cenarioSemProcessador = `
nome: Espera sem resposta
alvo: %s

dados:
  pedidos:
    gerar: { id: uuid }

carga:
  perfis:
    - constante: { taxa: 5/s, durante: 1s }

cenario:
  - kafka:
      topico: %s
      chave: "${pedidos.id}"
      valor: { pedido: "${pedidos.id}" }

  - aguardar:
      kafka: { topico: %s }
      chave: "${pedidos.id}"
      timeout: 2s
`

func TestMensagemQueNaoChegaViraTimeoutComExplicacao(t *testing.T) {
	brokers := brokerKafka(t)
	entrada := topico(t, brokers, "sem-processador", 1)
	saida := topico(t, brokers, "sem-processador-saida", 1)

	documento, _ := executar(t, fmt.Sprintf(cenarioSemProcessador, brokers, entrada, saida))

	var espera metrica.ResultadoDePasso
	for _, passo := range documento.Passos {
		if strings.HasPrefix(passo.Nome, "aguardar ") {
			espera = passo
		}
	}
	if espera.ErrosPorClasse["timeout"] == 0 {
		t.Fatalf("sem processador, a espera precisa virar timeout: %+v detalhes=%+v", espera.ErrosPorClasse, espera.Detalhes)
	}
	var explicou bool
	for detalhe := range espera.Detalhes {
		if strings.Contains(detalhe, "nao chegou em") && strings.Contains(detalhe, saida) {
			explicou = true
		}
	}
	if !explicou {
		t.Errorf("o detalhe precisa dizer o que era esperado e onde: %+v", espera.Detalhes)
	}
	if documento.Jornada.Completas != 0 {
		t.Error("jornada sem a mensagem de volta nao pode contar como completa")
	}
}

const cenarioAMQP = `
nome: Cadeia AMQP
alvo: %s

dados:
  pedidos:
    gerar: { id: uuid }

carga:
  perfis:
    - constante: { taxa: 50/s, durante: 2s }

cenario:
  - amqp:
      fila: %s
      identidade: "${pedidos.id}"
      corpo: { pedido: "${pedidos.id}" }

  - aguardar:
      amqp: { fila: %s, url: %s }
      chave: "${pedidos.id}"
      timeout: 10s

slo:
  - global: { erros: < 1 }
`

func TestAMQPPublicaEEsperaNaMesmaFila(t *testing.T) {
	endereco := brokerAMQP(t)
	fila := fmt.Sprintf("braunrate-teste-%d", time.Now().UnixNano())

	documento, veredito := executar(t, fmt.Sprintf(cenarioAMQP, endereco, fila, fila, endereco))
	if !veredito.Passou {
		t.Fatalf("a fila deveria fechar sem erro: %s", veredito.Frase)
	}
	if documento.Jornada.Completas == 0 {
		t.Fatal("nenhuma mensagem publicada voltou pela fila")
	}
	var publicacao bool
	for _, passo := range documento.Passos {
		if passo.Nome == "amqp publicar "+fila {
			publicacao = true
			if passo.Latencia.P50 <= 0 {
				t.Error("publicacao com confirmacao precisa ter latencia medida")
			}
		}
	}
	if !publicacao {
		t.Errorf("faltou a linha da publicacao no relatorio: %+v", documento.Passos)
	}
}
