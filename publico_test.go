package braunrate_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// O pacote dsl sempre foi publico e inutil de fora do modulo: ele devolve um
// cenario que so o proprio modulo conseguia executar, porque motor, SLO e
// relatorio estao em internal/. Metade da promessa da Fase 6 — o time versiona
// o teste junto do servico, em Go — nao era entregavel.
//
// Este teste compila um modulo de fora contra a superficie publica. Ele existe
// porque a alternativa e publicar uma API e descobrir que ela nao serve quando
// alguem de fora tentar — que ja aconteceu quatro vezes neste projeto com
// exemplo publicado que nunca foi executado.
func TestAModuleOutsideThisOneCompilesAgainstThePublicSurface(t *testing.T) {
	root, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("não consegui achar a raiz do módulo: %v", err)
	}
	outside := t.TempDir()

	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(outside, name), []byte(content), 0o600); err != nil {
			t.Fatalf("não consegui escrever %s: %v", name, err)
		}
	}

	write("go.mod", `module exemplo.com/servico

go 1.26.6

require github.com/Diegobraun/braunrate v0.0.0

replace github.com/Diegobraun/braunrate => `+root+"\n")

	write("main.go", `package main

import (
	"context"
	"os"
	"time"

	"github.com/Diegobraun/braunrate"
	"github.com/Diegobraun/braunrate/dsl"
)

func main() {
	spec, err := dsl.New("Cenário de fora do módulo").
		Target("http://127.0.0.1:8080").
		Steady(dsl.PerSecond(50), 2*time.Second).
		Step(dsl.GET("/pedidos/1"), dsl.Name("consultar pedido"), dsl.CheckStatus(200)).
		SLO("consultar pedido", "p95", "< 500ms").
		OverallSLO("erros", "< 1").
		Build()
	if err != nil {
		os.Exit(2)
	}
	resultado, err := braunrate.Run(context.Background(), spec, braunrate.Options{Version: "de-fora"})
	if err != nil {
		os.Exit(2)
	}
	_ = braunrate.Summary(os.Stdout, resultado)
	_ = braunrate.Passed(resultado)
	os.Exit(braunrate.ExitCode(resultado))
}
`)

	// go.sum do modulo de fora sai do daqui: as dependencias sao as mesmas, e
	// copiar evita rede no teste.
	sum, err := os.ReadFile(filepath.Join(root, "go.sum"))
	if err != nil {
		t.Fatalf("não consegui ler o go.sum: %v", err)
	}
	write("go.sum", string(sum))

	build := exec.Command("go", "build", "-o", filepath.Join(outside, "servico"), ".")
	build.Dir = outside
	build.Env = append(os.Environ(), "GOFLAGS=-mod=mod")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("um módulo de fora não compila contra a superficie publica: %v\n%s", err, output)
	}
}

// Um import de internal/ de fora do modulo nao compila, e essa e a fronteira que
// decide o que e contrato. Se este teste passar a compilar, alguma coisa saiu de
// internal/ sem decisao registrada.
func TestTheInternalPackagesStayOutOfReach(t *testing.T) {
	root, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("não consegui achar a raiz do módulo: %v", err)
	}
	outside := t.TempDir()

	files := map[string]string{
		"go.mod": "module exemplo.com/servico\n\ngo 1.26.6\n\nrequire github.com/Diegobraun/braunrate v0.0.0\n\nreplace github.com/Diegobraun/braunrate => " + root + "\n",
		"main.go": `package main

import _ "github.com/Diegobraun/braunrate/internal/engine"

func main() {}
`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(outside, name), []byte(content), 0o600); err != nil {
			t.Fatalf("não consegui escrever %s: %v", name, err)
		}
	}

	build := exec.Command("go", "build", ".")
	build.Dir = outside
	build.Env = append(os.Environ(), "GOFLAGS=-mod=mod")
	output, err := build.CombinedOutput()
	if err == nil {
		t.Fatal("um módulo de fora conseguiu importar internal/: a fronteira do contrato publico caiu")
	}
	if !strings.Contains(string(output), "internal") {
		t.Fatalf("a compilacao falhou por outro motivo:\n%s", output)
	}
}
