package testsupport

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type graphQLRequest struct {
	Query         string         `json:"query"`
	OperationName string         `json:"operationName"`
	Vars          map[string]any `json:"variables"`
}

// The target returns a GraphQL error with status 200 on purpose: that is how
// the error arrives in production, and exactly the case a tool that only looks
// at HTTP status counts as a success.
func (server *Server) handleGraphQL(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var order graphQLRequest
	if err := json.NewDecoder(r.Body).Decode(&order); err != nil {
		serveGraphQL(w, "", `{"errors":[{"message":"body is not JSON","extensions":{"code":"BAD_REQUEST"}}]}`)
		return
	}

	operation := order.OperationName
	if operation == "" {
		operation = operationName(order.Query)
	}
	identifier := text(order.Vars["id"])

	switch operation {
	case "LookUpOrder":
		if strings.HasSuffix(identifier, "7") {
			serveGraphQL(w, operation, fmt.Sprintf(
				`{"data":{"order":null},"errors":[{"message":"order %s not found","path":["order"],"extensions":{"code":"NOT_FOUND"}}]}`,
				identifier))
			return
		}
		serveGraphQL(w, operation, fmt.Sprintf(
			`{"data":{"order":{"id":%q,"status":"OPEN","lastInvoice":{"id":"f-%s","amount":199.90,"status":"OPEN"}}}}`,
			identifier, identifier))
	case "PayInvoice":
		invoice := text(order.Vars["invoice"])
		serveGraphQL(w, operation, fmt.Sprintf(
			`{"data":{"payInvoice":{"id":%q,"status":"PAID","paidAt":"2026-08-15T00:00:00Z"}}}`, invoice))
	default:
		serveGraphQL(w, operation, fmt.Sprintf(
			`{"errors":[{"message":"operation %q does not exist in the schema","extensions":{"code":"GRAPHQL_VALIDATION_FAILED"}}]}`, operation))
	}
}

func serveGraphQL(w http.ResponseWriter, operation, body string) {
	w.Header().Set("Content-Type", "application/json")
	if operation != "" {
		w.Header().Set("X-Operation", operation)
	}
	_, _ = fmt.Fprint(w, body)
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
