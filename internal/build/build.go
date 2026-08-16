// Package build guarda a identidade do binario: quem pergunta "que versao
// produziu este numero" precisa da resposta no proprio artefato, nao no
// repositorio de onde ele saiu.
package build

// Preenchidos por -ldflags -X na hora de publicar. Os valores aqui sao os de um
// binario compilado a mao, e dizem isso: um resultado gravado por um binario
// "dev" nao e comparavel com um resultado de release, e a comparacao entre
// execucoes de versoes diferentes sai sem veredito por causa disso.
var (
	Version = "dev"
	Commit  = "desconhecido"
	Date    = "desconhecido"
)
