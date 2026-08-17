package data_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Diegobraun/braunrate/internal/data"
	"github.com/Diegobraun/braunrate/internal/scenario"
)

func writeCSV(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	content := "id,nome\n1,ana\n2,bruno\n3,carla\n"
	if err := os.WriteFile(filepath.Join(root, "assinantes.csv"), []byte(content), 0o644); err != nil {
		t.Fatalf("não consegui escrever o csv: %v", err)
	}
	return root
}

func TestCircularConsumeWrapsAround(t *testing.T) {
	root := writeCSV(t)
	source, err := data.Open(scenario.DataSource{Name: "assinantes", File: "assinantes.csv",
		Consume: scenario.ConsumeCircular}, root)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	var read []string
	for i := 0; i < 5; i++ {
		record, err := source.Next(int64(i))
		if err != nil {
			t.Fatalf("iteração %d: %v", i, err)
		}
		read = append(read, record["assinantes.id"])
	}
	if got := strings.Join(read, ","); got != "1,2,3,1,2" {
		t.Errorf("consumo circular = %q, esperado 1,2,3,1,2", got)
	}
}

func TestSequentialConsumeWarnsWhenDataRunsOut(t *testing.T) {
	root := writeCSV(t)
	source, _ := data.Open(scenario.DataSource{Name: "assinantes", File: "assinantes.csv",
		Consume: scenario.ConsumeSequential}, root)

	for i := 0; i < 3; i++ {
		if _, err := source.Next(int64(i)); err != nil {
			t.Fatalf("iteração %d deveria funcionar: %v", i, err)
		}
	}
	_, err := source.Next(3)
	if err == nil {
		t.Fatal("a quarta leitura deveria acusar fim dos dados")
	}
	if !strings.Contains(err.Error(), "circular") {
		t.Errorf("a mensagem precisa ensinar a saída: %v", err)
	}
	if !source.Exhausted() {
		t.Error("a fonte deveria estar marcada como esgotada")
	}
}

func TestUniquePerUserConsumeGivesFixedRowPerUser(t *testing.T) {
	root := writeCSV(t)
	source, _ := data.Open(scenario.DataSource{Name: "assinantes", File: "assinantes.csv",
		Consume: scenario.ConsumeUniquePerUser}, root)

	first, _ := source.Next(7)
	second, _ := source.Next(7)
	another, _ := source.Next(8)

	if first["assinantes.id"] != second["assinantes.id"] {
		t.Error("o mesmo usuário virtual precisa receber sempre a mesma linha")
	}
	if first["assinantes.id"] == another["assinantes.id"] {
		t.Error("usuários virtuais diferentes deveriam receber linhas diferentes")
	}
}

func TestRandomConsumeWithSameSeedRepeatsSequence(t *testing.T) {
	root := writeCSV(t)
	sequence := func() []string {
		source, _ := data.Open(scenario.DataSource{Name: "assinantes", File: "assinantes.csv",
			Consume: scenario.ConsumeRandom, Seed: 42}, root)
		var read []string
		for i := 0; i < 10; i++ {
			record, _ := source.Next(int64(i))
			read = append(read, record["assinantes.id"])
		}
		return read
	}
	first, again := strings.Join(sequence(), ","), strings.Join(sequence(), ",")
	if first != again {
		t.Error("mesma semente precisa produzir a mesma sequência; sem isso a execução não e reproduzivel")
	}
}

func TestSyntheticGenerationIsReproducibleBySeed(t *testing.T) {
	source := scenario.DataSource{Name: "pedidos", Seed: 7, Fields: generators(map[string]string{
		"id":     "uuid",
		"valor":  "number(10,500)",
		"ordem":  "sequence",
		"quem":   "name",
		"email":  "email",
		"codigo": "text(6)",
	})}

	generate := func() []string {
		open, err := data.Open(source, "")
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		var lines []string
		for i := 0; i < 5; i++ {
			record, err := open.Next(int64(i))
			if err != nil {
				t.Fatalf("geracao falhou: %v", err)
			}
			lines = append(lines, record["pedidos.id"]+"|"+record["pedidos.valor"]+"|"+record["pedidos.ordem"])
		}
		return lines
	}

	first, second := generate(), generate()
	if strings.Join(first, ";") != strings.Join(second, ";") {
		t.Fatalf("geracao não reproduzivel:\n%v\n%v", first, second)
	}
	if first[0] == first[1] {
		t.Error("registros diferentes deveriam ter valores diferentes")
	}
}

func TestUnknownGeneratorTeachesValidOnes(t *testing.T) {
	source := scenario.DataSource{Name: "x", Fields: generators(map[string]string{"a": "telefone"})}
	open, err := data.Open(source, "")
	if err != nil {
		t.Fatalf("abrir deveria funcionar: %v", err)
	}
	_, err = open.Next(0)
	if err == nil || !strings.Contains(err.Error(), "available: uuid") {
		t.Fatalf("esperava lista de geradores válidos, recebeu %v", err)
	}
}

func TestMissingFileExplainsProblem(t *testing.T) {
	_, err := data.Open(scenario.DataSource{Name: "x", File: "nao-existe.csv"}, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "nao-existe.csv") {
		t.Fatalf("esperava mensagem citando o arquivo, recebeu %v", err)
	}
}

func generators(recipes map[string]string) map[string]scenario.Generator {
	fields := make(map[string]scenario.Generator, len(recipes))
	for name, recipe := range recipes {
		fields[name] = scenario.ParseGenerator(recipe)
	}
	return fields
}
