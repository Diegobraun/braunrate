package importer

import (
	"strings"
	"testing"
)

const sampleHAR = `{
  "log": {
    "entries": [
      {
        "_resourceType": "xhr",
        "request": {
          "method": "POST",
          "url": "https://api.example.com/orders?ref=1",
          "headers": [
            {"name": "Authorization", "value": "Bearer secret-token"},
            {"name": ":authority", "value": "api.example.com"},
            {"name": "Content-Length", "value": "12"}
          ],
          "postData": {"mimeType": "application/json", "text": "{\"id\":1}"}
        },
        "response": {"status": 201, "content": {"mimeType": "application/json"}}
      },
      {
        "_resourceType": "image",
        "request": {"method": "GET", "url": "https://api.example.com/logo.png", "headers": []},
        "response": {"status": 200, "content": {"mimeType": "image/png"}}
      },
      {
        "_resourceType": "xhr",
        "request": {"method": "GET", "url": "https://api.example.com/orders/1", "headers": []},
        "response": {"status": 200, "content": {"mimeType": "application/json"}}
      }
    ]
  }
}`

func TestFromHARKeepsAPICallsAndMasksSecrets(t *testing.T) {
	result, err := FromHAR([]byte(sampleHAR))
	if err != nil {
		t.Fatalf("FromHAR: %v", err)
	}
	yaml := result.YAML
	if !strings.Contains(yaml, "target: ${TARGET:-https://api.example.com}") {
		t.Fatalf("target not set from the commonest origin:\n%s", yaml)
	}
	if strings.Contains(yaml, "logo.png") {
		t.Fatalf("the image asset was not left out:\n%s", yaml)
	}
	if !strings.Contains(yaml, "method: POST") || !strings.Contains(yaml, "path: /orders?ref=1") {
		t.Fatalf("the POST step is missing:\n%s", yaml)
	}
	if !strings.Contains(yaml, "http: GET /orders/1") {
		t.Fatalf("the GET xhr step is missing:\n%s", yaml)
	}
	if strings.Contains(yaml, "secret-token") {
		t.Fatalf("the bearer token leaked into the YAML:\n%s", yaml)
	}
	if strings.Contains(yaml, ":authority") {
		t.Fatalf("an HTTP/2 pseudo-header leaked into the YAML:\n%s", yaml)
	}
	found := false
	for _, warning := range result.Warnings {
		if strings.Contains(warning, "left out") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the assets left out were not declared in the warnings: %v", result.Warnings)
	}
}

func TestFromHARRejectsAllAssets(t *testing.T) {
	onlyAssets := `{"log":{"entries":[
      {"_resourceType":"image","request":{"method":"GET","url":"https://x/a.png","headers":[]},"response":{"status":200,"content":{"mimeType":"image/png"}}}
    ]}}`
	if _, err := FromHAR([]byte(onlyAssets)); err == nil {
		t.Fatal("a HAR with only assets should be rejected")
	}
}
