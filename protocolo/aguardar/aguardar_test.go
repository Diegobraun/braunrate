package aguardar_test

import (
	"strings"
	"testing"

	"github.com/Diegobraun/braunrate/protocolo"
	"github.com/Diegobraun/braunrate/protocolo/aguardar"
	"gopkg.in/yaml.v3"
)

func decodificar(t *testing.T, texto string) (protocolo.Configuracao, error) {
	t.Helper()
	var documento yaml.Node
	if err := yaml.Unmarshal([]byte(texto), &documento); err != nil {
		t.Fatalf("yaml invalido no teste: %v", err)
	}
	return aguardar.Novo(protocolo.OpcoesPadrao()).Decodificar(documento.Content[0])
}

func TestChaveDeAgregacaoEhODestinoEsperado(t *testing.T) {
	configuracao, err := decodificar(t, "kafka: { topico: pedidos-processados }\nchave: \"${pedidos.id}\"\n")
	if err != nil {
		t.Fatalf("nao decodificou: %v", err)
	}
	if configuracao.ChaveDeAgregacao() != "aguardar pedidos-processados" {
		t.Errorf("chave = %q", configuracao.ChaveDeAgregacao())
	}
}

// Sem valor de correlacao, qualquer mensagem serviria: a medicao passaria a
// medir o consumidor mais rapido do topico, e nao a mensagem daquela iteracao.
func TestEsperarSemCorrelacaoEhRecusado(t *testing.T) {
	_, err := decodificar(t, "kafka: { topico: pedidos-processados }\n")
	if err == nil {
		t.Fatal("esperava erro")
	}
	if !strings.Contains(err.Error(), "mediria o consumidor mais rapido") {
		t.Errorf("a mensagem precisa dizer por que isso invalida a medicao: %q", err.Error())
	}
}

func TestTimeoutTemPadraoEhPodeSerDeclarado(t *testing.T) {
	configuracao, err := decodificar(t, "kafka: { topico: t }\nchave: x\n")
	if err != nil {
		t.Fatalf("nao decodificou: %v", err)
	}
	descricao := strings.Join(configuracao.(protocolo.ConfiguracaoDescritivel).Descrever(), " ")
	if !strings.Contains(descricao, "30s") {
		t.Errorf("faltou o timeout padrao na descricao: %s", descricao)
	}

	comTimeout, err := decodificar(t, "kafka: { topico: t }\nchave: x\ntimeout: 90s\n")
	if err != nil {
		t.Fatalf("nao decodificou: %v", err)
	}
	if !strings.Contains(strings.Join(comTimeout.(protocolo.ConfiguracaoDescritivel).Descrever(), " "), "1m30s") {
		t.Error("timeout declarado nao apareceu na descricao")
	}
}

func TestSemEnderecoEnsinaOndeDeclarar(t *testing.T) {
	configuracao, err := decodificar(t, "kafka: { topico: t }\nchave: x\n")
	if err != nil {
		t.Fatalf("nao decodificou: %v", err)
	}
	resposta := aguardar.Novo(protocolo.OpcoesPadrao()).Executar(t.Context(), protocolo.Requisicao{Configuracao: configuracao})
	if resposta.Classe != protocolo.ErroDeConfigacao || !strings.Contains(resposta.Detalhe, "brokers") {
		t.Errorf("classe = %q, detalhe = %q", resposta.Classe, resposta.Detalhe)
	}
}
