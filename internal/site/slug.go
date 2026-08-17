package site

import "strings"

// O identificador da secao e escrito aqui, e nao deixado para o goldmark: ele
// descarta letra acentuada em vez de transliterar, e "propositos" virava
// "propsitos" na barra de endereco. As letras acentuadas estao aqui porque sao
// a entrada da transliteracao, e nao texto que alguem le.
var accents = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a",
	"é", "e", "ê", "e", "è", "e", "í", "i", "î", "i", "ì", "i",
	"ó", "o", "ô", "o", "õ", "o", "ò", "o", "ö", "o",
	"ú", "u", "û", "u", "ù", "u", "ü", "u", "ç", "c", "ñ", "n",
)

func slugify(text string) string {
	var written strings.Builder
	previousDash := false
	for _, character := range accents.Replace(strings.ToLower(plain(text))) {
		switch {
		case character >= 'a' && character <= 'z', character >= '0' && character <= '9':
			written.WriteRune(character)
			previousDash = false
		case !previousDash && written.Len() > 0:
			written.WriteRune('-')
			previousDash = true
		}
	}
	return strings.TrimSuffix(written.String(), "-")
}
