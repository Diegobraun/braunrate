package testsupport

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type pedidoGraphQL struct {
	Query         string         `json:"query"`
	OperationName string         `json:"operationName"`
	Vars          map[string]any `json:"variables"`
}

// O alvo devolve erro de GraphQL com status 200 de proposito: e assim que o
// erro chega em producao, e e exatamente o caso que uma ferramenta que so olha
// o status HTTP contabiliza como sucesso.
func (s *Server) handleGraphQL(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var order pedidoGraphQL
	if err := json.NewDecoder(r.Body).Decode(&order); err != nil {
		serveGraphQL(w, "", `{"errors":[{"message":"corpo nao e JSON","extensions":{"code":"BAD_REQUEST"}}]}`)
		return
	}

	operation := order.OperationName
	if operation == "" {
		operation = operationName(order.Query)
	}
	identifier := text(order.Vars["id"])

	switch operation {
	case "ConsultarPedido":
		if strings.HasSuffix(identifier, "7") {
			serveGraphQL(w, operation, fmt.Sprintf(
				`{"data":{"pedido":null},"errors":[{"message":"pedido %s nao encontrado","path":["pedido"],"extensions":{"code":"NOT_FOUND"}}]}`,
				identifier))
			return
		}
		serveGraphQL(w, operation, fmt.Sprintf(
			`{"data":{"pedido":{"id":%q,"status":"ABERTO","ultimaFatura":{"id":"f-%s","valor":199.90,"status":"ABERTA"}}}}`,
			identifier, identifier))
	case "PagarFatura":
		invoice := text(order.Vars["fatura"])
		serveGraphQL(w, operation, fmt.Sprintf(
			`{"data":{"pagarFatura":{"id":%q,"status":"PAGA","pagoEm":"2026-08-15T00:00:00Z"}}}`, invoice))
	default:
		serveGraphQL(w, operation, fmt.Sprintf(
			`{"errors":[{"message":"operacao %q nao existe no schema","extensions":{"code":"GRAPHQL_VALIDATION_FAILED"}}]}`, operation))
	}
}

func serveGraphQL(w http.ResponseWriter, operation, body string) {
	w.Header().Set("Content-Type", "application/json")
	if operation != "" {
		w.Header().Set("X-Operacao", operation)
	}
	fmt.Fprint(w, body)
}

func operationName(query string) string {
	fields := strings.Fields(query)
	for index, field := range fields {
		if field != "query" && field != "mutation" {
			continue
		}
		if index+1 >= len(fields) {
			return ""
		}
		name, _, _ := strings.Cut(fields[index+1], "(")
		name, _, _ = strings.Cut(name, "{")
		return name
	}
	return ""
}

func text(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}
