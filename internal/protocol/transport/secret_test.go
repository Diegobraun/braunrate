package transport_test

import (
	"strings"
	"testing"

	"github.com/Diegobraun/braunrate/internal/protocol/transport"
)

// A depuracao vai parar em ticket e em captura de tela, e um cookie de sessao
// abre a conta de quem gravou tanto quanto um Bearer. O nome do par fica: sem
// ele nao da para saber o que o passo esta mandando.
func TestSessionCookieIsCutLikeTheBearerAlreadyWas(t *testing.T) {
	masked := transport.MaskSecret("Cookie", "sessao=eb5b94f531fa41c9ad8e8a4953b59b4b; idioma=pt")

	if strings.Contains(masked, "eb5b94f531fa41c9ad8e8a4953b59b4b") {
		t.Fatalf("o cookie de sessão saiu inteiro: %q", masked)
	}
	if !strings.HasPrefix(masked, "sessao=eb5b94… (32 characters)") {
		t.Fatalf("o corte não seguiu a forma que já existia para o Bearer: %q", masked)
	}
	if !strings.Contains(masked, "idioma=***") {
		t.Fatalf("o par curto também precisa sair cortado: %q", masked)
	}
}

func TestApiKeyHeaderIsCutToo(t *testing.T) {
	if masked := transport.MaskSecret("X-API-Key", "chave-secreta-de-producao"); strings.Contains(masked, "secreta") {
		t.Fatalf("a chave de API saiu inteira: %q", masked)
	}
}

func TestOrdinaryHeaderIsNotTouched(t *testing.T) {
	if masked := transport.MaskSecret("Content-Type", "application/json"); masked != "application/json" {
		t.Fatalf("o cabecalho comum foi mexido: %q", masked)
	}
}
