package importer_test

import (
	"strings"
	"testing"

	"github.com/Diegobraun/braunrate/internal/importer"
	"github.com/Diegobraun/braunrate/internal/scenario"

	_ "github.com/Diegobraun/braunrate/internal/protocol/amqp"
	_ "github.com/Diegobraun/braunrate/internal/protocol/graphql"
	_ "github.com/Diegobraun/braunrate/internal/protocol/http"
	_ "github.com/Diegobraun/braunrate/internal/protocol/kafka"
	_ "github.com/Diegobraun/braunrate/internal/protocol/wait"
)

// O esqueleto e o unico caminho de entrada para quem comeca de uma pasta vazia,
// e ensinava um protocolo de cinco. Quem precisava de mensageria descartava a
// saida inteira e escrevia o arquivo a mao, com a forma que so o codigo-fonte
// ou os exemplos do repositorio mostram.
func TestSkeletonShowsEveryProtocolThatIsCompiledIn(t *testing.T) {
	skeleton := importer.Skeleton()

	for _, protocol := range []string{"http", "graphql", "kafka", "amqp", "aguardar"} {
		if !strings.Contains(skeleton, protocol) {
			t.Errorf("o esqueleto não mostra a forma do passo %q", protocol)
		}
	}
}

// O que o esqueleto mostra descomentado precisa continuar sendo um cenario que
// roda: ele e a primeira coisa que a pessoa executa.
func TestSkeletonIsAValidScenario(t *testing.T) {
	if _, err := scenario.Parse([]byte(importer.Skeleton())); err != nil {
		t.Fatalf("o esqueleto que a ferramenta escreve não passa no próprio parser: %v", err)
	}
}

// Forma comentada que nao passa no parser ensina errado justamente a quem nao
// tem outra referencia. A primeira versao deste bloco escrevia "valor" no passo
// amqp, que usa "corpo".
func TestCommentedProtocolShapesParse(t *testing.T) {
	document := `nome: formas
alvo: 127.0.0.1:9092
dados:
  assinantes: { gerar: { id: uuid } }
carga:
  perfis:
    - patamar: { taxa: 20/s, durante: 1m }
` + importer.ProtocolShapes()

	if _, err := scenario.Parse([]byte(document)); err != nil {
		t.Fatalf("a forma que o esqueleto ensina não passa no parser: %v", err)
	}
}
