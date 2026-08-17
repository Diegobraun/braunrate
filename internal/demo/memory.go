package demo

import (
	"os"
	"path/filepath"
)

// A explicacao ao lado de cada numero e o que faz a demonstracao servir para
// quem nunca rodou um teste de carga, e e ruido para quem ja rodou. Uma bandeira
// nao resolve: quem se irrita com a explicacao e justamente quem nao vai ler a
// ajuda para descobrir que a bandeira existe.
//
// Este e o unico arquivo que a ferramenta escreve fora do diretorio de trabalho,
// e ele guarda um fato so: a demonstracao ja rodou aqui. Se o diretorio nao
// existir ou nao for gravavel, a explicacao volta a aparecer — errar para o lado
// de explicar demais custa rolagem, e errar para o outro deixa alguem sem
// entender o proprio relatorio.
const marker = "demo-was-seen"

var configDirectory = os.UserConfigDir

func firstTime() bool {
	path, err := markerPath()
	if err != nil {
		return true
	}
	_, err = os.Stat(path)
	return err != nil
}

func remember() {
	path, err := markerPath()
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(path, nil, 0o644)
}

func markerPath() (string, error) {
	directory, err := configDirectory()
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, "braunrate", marker), nil
}

// Sem isto o teste leria e escreveria a marca da propria maquina, e a segunda
// rodada da suite veria uma demonstracao diferente da primeira.
func rememberElsewhere(directory string) func() {
	previous := configDirectory
	configDirectory = func() (string, error) { return directory, nil }
	return func() { configDirectory = previous }
}
