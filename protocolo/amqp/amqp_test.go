package amqp_test

import (
	"strings"
	"testing"

	"github.com/Diegobraun/braunrate/protocolo"
	"github.com/Diegobraun/braunrate/protocolo/amqp"
	"gopkg.in/yaml.v3"
)

func decodificar(t *testing.T, texto string) (protocolo.Configuracao, error) {
	t.Helper()
	var documento yaml.Node
	if err := yaml.Unmarshal([]byte(texto), &documento); err != nil {
		t.Fatalf("yaml invalido no teste: %v", err)
	}
	return amqp.Novo(protocolo.OpcoesPadrao()).Decodificar(documento.Content[0])
}

func TestFilaSozinhaBastaEViraRota(t *testing.T) {
	configuracao, err := decodificar(t, "fila: pedidos\ncorpo: { id: 1 }\n")
	if err != nil {
		t.Fatalf("nao decodificou: %v", err)
	}
	if configuracao.ChaveDeAgregacao() != "amqp publicar pedidos" {
		t.Errorf("chave = %q", configuracao.ChaveDeAgregacao())
	}
}

func TestTrocaComRotaApareceNaChave(t *testing.T) {
	configuracao, err := decodificar(t, "troca: cobranca\nrota: pedido.criado\ncorpo: { id: 1 }\n")
	if err != nil {
		t.Fatalf("nao decodificou: %v", err)
	}
	if configuracao.ChaveDeAgregacao() != "amqp publicar cobranca/pedido.criado" {
		t.Errorf("chave = %q", configuracao.ChaveDeAgregacao())
	}
}

// Sem confirmacao, o tempo medido e o de escrever no socket: mediria a rede
// local, e nao o broker aceitando a mensagem.
func TestConfirmacaoEhOPadrao(t *testing.T) {
	configuracao, err := decodificar(t, "fila: pedidos\ncorpo: { id: 1 }\n")
	if err != nil {
		t.Fatalf("nao decodificou: %v", err)
	}
	descricao := strings.Join(configuracao.(protocolo.ConfiguracaoDescritivel).Descrever(), " ")
	if !strings.Contains(descricao, "espera confirmacao do broker") {
		t.Errorf("descricao = %s", descricao)
	}
}

func TestPassoSemDestinoOuSemCorpoEnsinaAFormaCerta(t *testing.T) {
	for nome, texto := range map[string]string{
		"sem destino": "corpo: { id: 1 }\n",
		"sem corpo":   "fila: pedidos\n",
	} {
		_, err := decodificar(t, texto)
		if err == nil {
			t.Fatalf("%s: esperava erro", nome)
		}
		if !strings.Contains(err.Error(), "- amqp:") {
			t.Errorf("%s: a mensagem precisa mostrar um exemplo: %q", nome, err.Error())
		}
	}
}
