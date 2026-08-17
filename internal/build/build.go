// Package build guarda a identidade do binario: quem pergunta "que versao
// produziu este numero" precisa da resposta no proprio artefato, nao no
// repositorio de onde ele saiu.
package build

// Preenchidos por -ldflags -X na hora de publicar. Os valores aqui sao os de um
// binario compilado a mao, e dizem isso: um resultado gravado por um binario
// "dev" is not comparable with a release result, and a comparison between runs
// of different versions comes out with no verdict because of it.
var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)
