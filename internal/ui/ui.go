// Package ui carrega a interface embarcada no proprio binario. Sem etapa de
// build para quem baixou o executavel: exigir node e bundler de quem so quer
// rodar um teste de carga seria o mesmo tipo de barreira que o binario unico
// existe para nao ter.
package ui

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed app
var files embed.FS

// O roteamento e por hash, entao endereco desconhecido devolve a pagina, nao 404.
func Handler() http.Handler {
	app, err := fs.Sub(files, "app")
	if err != nil {
		panic("interface embarcada quebrada: " + err.Error())
	}
	assets := http.FileServer(http.FS(app))
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/" || !strings.Contains(request.URL.Path, ".") {
			request = request.Clone(request.Context())
			request.URL.Path = "/"
		}
		assets.ServeHTTP(writer, request)
	})
}
