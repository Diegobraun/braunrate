package scenario

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ReadSeed le "semente: 42" e "semente: ${SEMENTE:-42}", devolvendo o valor que
// vai rodar e a variavel de ambiente de onde ele veio.
//
// Semente fixa no arquivo faz o CI rodar sempre o mesmo caso, e um caso que
// passa mil vezes nao prova mais nada depois da primeira. Deixar a semente vir
// do ambiente e o que permite variar; guardar de onde ela veio e o que permite
// voltar ao caso que falhou — sem isso, variar e so perder a execucao.
func ReadSeed(declared string) (int64, string, error) {
	text := strings.TrimSpace(declared)
	resolved := ExpandFromEnv(text)

	origin := ""
	if name := referencedEnvironmentName(text); name != "" {
		if _, present := os.LookupEnv(name); present {
			origin = name
		}
	}

	seed, err := strconv.ParseInt(strings.TrimSpace(resolved), 10, 64)
	if err != nil {
		if origin != "" {
			return 0, "", fmt.Errorf("invalid seed: $%s is %q and the seed has to be a whole number", origin, resolved)
		}
		return 0, "", fmt.Errorf("invalid seed: %q (use a whole number, or ${SEED:-42} to take it from the environment)", declared)
	}
	return seed, origin, nil
}

// SeedsFromEnvironment mapeia fonte para a variavel de ambiente que decidiu a
// semente dela. Fonte com semente escrita no arquivo fica de fora: nao ha o que
// exportar para repetir.
func SeedsFromEnvironment(spec Spec) map[string]string {
	origins := map[string]string{}
	for _, source := range spec.Data {
		if source.SeedFrom != "" {
			origins[source.Name] = source.SeedFrom
		}
	}
	if len(origins) == 0 {
		return nil
	}
	return origins
}

// A semente so muda o que sai de uma fonte que usa aleatoriedade: sintetica
// sempre, e CSV so quando o consumo e aleatorio. Publicar uma semente que nao
// muda nada seria ruido no bloco que a pessoa le para reproduzir.
func (source DataSource) UsesSeed() bool {
	return source.Synthetic() || source.Consume == ConsumeRandom
}

func referencedEnvironmentName(text string) string {
	parts := varPattern.FindStringSubmatch(text)
	if parts == nil {
		return ""
	}
	return parts[1]
}
