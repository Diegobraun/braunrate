package data_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Diegobraun/braunrate/internal/data"
	"github.com/Diegobraun/braunrate/internal/scenario"
)

func escreverCSV(t *testing.T) string {
	t.Helper()
	raiz := t.TempDir()
	conteudo := "id,nome\n1,ana\n2,bruno\n3,carla\n"
	if err := os.WriteFile(filepath.Join(raiz, "assinantes.csv"), []byte(conteudo), 0o644); err != nil {
		t.Fatalf("nao consegui escrever o csv: %v", err)
	}
	return raiz
}

func TestConsumoCircularVoltaAoInicio(t *testing.T) {
	raiz := escreverCSV(t)
	fonte, err := data.Abrir(scenario.FonteDeDados{Nome: "assinantes", Arquivo: "assinantes.csv",
		Consumo: scenario.ConsumoCircular}, raiz)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	var lidos []string
	for i := 0; i < 5; i++ {
		registro, err := fonte.Proximo(int64(i))
		if err != nil {
			t.Fatalf("iteracao %d: %v", i, err)
		}
		lidos = append(lidos, registro["assinantes.id"])
	}
	if got := strings.Join(lidos, ","); got != "1,2,3,1,2" {
		t.Errorf("consumo circular = %q, esperado 1,2,3,1,2", got)
	}
}

func TestConsumoSequencialAvisaQuandoOsDadosAcabam(t *testing.T) {
	raiz := escreverCSV(t)
	fonte, _ := data.Abrir(scenario.FonteDeDados{Nome: "assinantes", Arquivo: "assinantes.csv",
		Consumo: scenario.ConsumoSequencial}, raiz)

	for i := 0; i < 3; i++ {
		if _, err := fonte.Proximo(int64(i)); err != nil {
			t.Fatalf("iteracao %d deveria funcionar: %v", i, err)
		}
	}
	_, err := fonte.Proximo(3)
	if err == nil {
		t.Fatal("a quarta leitura deveria acusar fim dos dados")
	}
	if !strings.Contains(err.Error(), "circular") {
		t.Errorf("a mensagem precisa ensinar a saida: %v", err)
	}
	if !fonte.Esgotada() {
		t.Error("a fonte deveria estar marcada como esgotada")
	}
}

func TestConsumoUnicoPorUsuarioDaLinhaFixaParaCadaUsuario(t *testing.T) {
	raiz := escreverCSV(t)
	fonte, _ := data.Abrir(scenario.FonteDeDados{Nome: "assinantes", Arquivo: "assinantes.csv",
		Consumo: scenario.ConsumoUnicoPorUsuario}, raiz)

	primeira, _ := fonte.Proximo(7)
	segunda, _ := fonte.Proximo(7)
	outra, _ := fonte.Proximo(8)

	if primeira["assinantes.id"] != segunda["assinantes.id"] {
		t.Error("o mesmo usuario virtual precisa receber sempre a mesma linha")
	}
	if primeira["assinantes.id"] == outra["assinantes.id"] {
		t.Error("usuarios virtuais diferentes deveriam receber linhas diferentes")
	}
}

func TestConsumoAleatorioComMesmaSementeRepeteASequencia(t *testing.T) {
	raiz := escreverCSV(t)
	sequencia := func() []string {
		fonte, _ := data.Abrir(scenario.FonteDeDados{Nome: "assinantes", Arquivo: "assinantes.csv",
			Consumo: scenario.ConsumoAleatorio, Semente: 42}, raiz)
		var lidos []string
		for i := 0; i < 10; i++ {
			registro, _ := fonte.Proximo(int64(i))
			lidos = append(lidos, registro["assinantes.id"])
		}
		return lidos
	}
	if strings.Join(sequencia(), ",") != strings.Join(sequencia(), ",") {
		t.Error("mesma semente precisa produzir a mesma sequencia; sem isso a execucao nao e reproduzivel")
	}
}

func TestGeracaoSinteticaEhReproduzivelPelaSemente(t *testing.T) {
	fonte := scenario.FonteDeDados{Nome: "pedidos", Semente: 7, Campos: map[string]string{
		"id":     "uuid",
		"valor":  "numero(10,500)",
		"ordem":  "sequencia",
		"quem":   "nome",
		"email":  "email",
		"codigo": "texto(6)",
	}}

	gerar := func() []string {
		aberta, err := data.Abrir(fonte, "")
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		var linhas []string
		for i := 0; i < 5; i++ {
			registro, err := aberta.Proximo(int64(i))
			if err != nil {
				t.Fatalf("geracao falhou: %v", err)
			}
			linhas = append(linhas, registro["pedidos.id"]+"|"+registro["pedidos.valor"]+"|"+registro["pedidos.ordem"])
		}
		return linhas
	}

	primeira, segunda := gerar(), gerar()
	if strings.Join(primeira, ";") != strings.Join(segunda, ";") {
		t.Fatalf("geracao nao reproduzivel:\n%v\n%v", primeira, segunda)
	}
	if primeira[0] == primeira[1] {
		t.Error("registros diferentes deveriam ter valores diferentes")
	}
}

func TestGeradorDesconhecidoEnsinaOsValidos(t *testing.T) {
	fonte := scenario.FonteDeDados{Nome: "x", Campos: map[string]string{"a": "telefone"}}
	aberta, err := data.Abrir(fonte, "")
	if err != nil {
		t.Fatalf("abrir deveria funcionar: %v", err)
	}
	_, err = aberta.Proximo(0)
	if err == nil || !strings.Contains(err.Error(), "use uuid") {
		t.Fatalf("esperava lista de geradores validos, recebeu %v", err)
	}
}

func TestArquivoInexistenteExplicaOProblema(t *testing.T) {
	_, err := data.Abrir(scenario.FonteDeDados{Nome: "x", Arquivo: "nao-existe.csv"}, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "nao-existe.csv") {
		t.Fatalf("esperava mensagem citando o arquivo, recebeu %v", err)
	}
}
