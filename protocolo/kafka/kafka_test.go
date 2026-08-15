package kafka_test

import (
	"strings"
	"testing"

	"github.com/Diegobraun/braunrate/protocolo"
	"github.com/Diegobraun/braunrate/protocolo/kafka"
	"gopkg.in/yaml.v3"
)

func decodificar(t *testing.T, texto string) (protocolo.Configuracao, error) {
	t.Helper()
	var documento yaml.Node
	if err := yaml.Unmarshal([]byte(texto), &documento); err != nil {
		t.Fatalf("yaml invalido no teste: %v", err)
	}
	return kafka.Novo(protocolo.OpcoesPadrao()).Decodificar(documento.Content[0])
}

// O topico e o fluxo de negocio; o broker e infraestrutura. Quem le o relatorio
// precisa saber qual fluxo ficou lento.
func TestChaveDeAgregacaoEhOTopico(t *testing.T) {
	configuracao, err := decodificar(t, "topico: pedidos\nchave: \"1\"\nvalor: { id: 1 }\n")
	if err != nil {
		t.Fatalf("nao decodificou: %v", err)
	}
	if configuracao.ChaveDeAgregacao() != "kafka produzir pedidos" {
		t.Errorf("chave = %q", configuracao.ChaveDeAgregacao())
	}
}

func TestChaveDaMensagemEhResolvidaPorIteracao(t *testing.T) {
	configuracao, err := decodificar(t, "topico: pedidos\nchave: \"${pedidos.id}\"\nvalor: { id: \"${pedidos.id}\" }\n")
	if err != nil {
		t.Fatalf("nao decodificou: %v", err)
	}
	resolvida := configuracao.Resolver(func(texto string) string {
		return strings.ReplaceAll(texto, "${pedidos.id}", "abc-123")
	})
	descricao := strings.Join(resolvida.(protocolo.ConfiguracaoDescritivel).Descrever(), " ")
	if !strings.Contains(descricao, "abc-123") {
		t.Errorf("chave e valor precisam ser resolvidos por iteracao: %s", descricao)
	}
}

func TestPassoSemTopicoOuSemValorEnsinaAFormaCerta(t *testing.T) {
	casos := map[string]string{
		"sem topico": "valor: { id: 1 }\n",
		"sem valor":  "topico: pedidos\n",
	}
	for nome, texto := range casos {
		_, err := decodificar(t, texto)
		if err == nil {
			t.Fatalf("%s: esperava erro", nome)
		}
		if !strings.Contains(err.Error(), "- kafka:") {
			t.Errorf("%s: a mensagem precisa mostrar um exemplo: %q", nome, err.Error())
		}
	}
}

func TestAcksDesconhecidoListaAsOpcoes(t *testing.T) {
	_, err := decodificar(t, "topico: pedidos\nvalor: { id: 1 }\nacks: talvez\n")
	if err == nil || !strings.Contains(err.Error(), "todos, lider ou nenhum") {
		t.Errorf("erro = %v", err)
	}
}

func TestSemBrokerEnsinaOndeDeclarar(t *testing.T) {
	configuracao, err := decodificar(t, "topico: pedidos\nvalor: { id: 1 }\n")
	if err != nil {
		t.Fatalf("nao decodificou: %v", err)
	}
	resposta := kafka.Novo(protocolo.OpcoesPadrao()).Executar(t.Context(), protocolo.Requisicao{Configuracao: configuracao})
	if resposta.Classe != protocolo.ErroDeConfigacao {
		t.Fatalf("classe = %q", resposta.Classe)
	}
	if !strings.Contains(resposta.Detalhe, "kafka://") {
		t.Errorf("detalhe = %q", resposta.Detalhe)
	}
}
