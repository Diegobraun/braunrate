package server

import (
	"encoding/json"
	"net/http"
	"sort"
	"sync"

	"github.com/Diegobraun/braunrate/internal/scenario"
)

// A interface precisa dar valor ao ${TOKEN} do cenario sem escrever o segredo no
// arquivo, que vai para o repositorio. O valor fica aqui, na memoria do processo,
// e some quando o servidor reinicia — o mesmo ciclo de vida de uma variavel de
// ambiente num shell. Nada disto vai para o disco. Ver ADR 0021.
type environment struct {
	mutex  sync.RWMutex
	values map[string]string
}

func newEnvironment() *environment {
	return &environment{values: map[string]string{}}
}

func (store *environment) snapshot() map[string]string {
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	if len(store.values) == 0 {
		return nil
	}
	copied := make(map[string]string, len(store.values))
	for name, value := range store.values {
		copied[name] = value
	}
	return copied
}

func (store *environment) set(incoming map[string]string) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	for name, value := range incoming {
		if value == "" {
			delete(store.values, name)
			continue
		}
		store.values[name] = value
	}
}

func (store *environment) names() []string {
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	names := make([]string, 0, len(store.values))
	for name := range store.values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// setEnvironment recebe um mapa nome→valor e guarda em memoria. So aceita nomes
// que o cenario referencia como variavel de ambiente (MAIUSCULA), a mesma
// convencao do ${TOKEN}, para nao virar um armazenamento de qualquer coisa.
func (server *Server) setEnvironment(writer http.ResponseWriter, request *http.Request) {
	var incoming map[string]string
	if err := json.NewDecoder(http.MaxBytesReader(writer, request.Body, maxScenarioBytes)).Decode(&incoming); err != nil {
		writeProblem(writer, http.StatusBadRequest, "the body has to be a JSON object of name to value")
		return
	}
	for name := range incoming {
		if !scenario.IsEnvironmentName(name) {
			writeProblem(writer, http.StatusBadRequest,
				"the name "+name+" is not an environment reference; use the uppercase name the scenario writes inside ${…}")
			return
		}
	}
	server.environment.set(incoming)
	// A resposta nunca devolve valor: so os nomes preenchidos, para a tela poder
	// mostrar "TOKEN definido" sem reimprimir o segredo.
	writeJSON(writer, http.StatusOK, map[string]any{"provided": server.environment.names()})
}

func (server *Server) getEnvironment(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]any{"provided": server.environment.names()})
}
