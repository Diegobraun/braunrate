package site

// O site tem duas linguas e uma fonte. O guia em ingles e o texto que vale; o
// em portugues declara de qual arquivo ele saiu e com que conteudo, e a build
// avisa quando o original andou e a traducao ficou parada.
type Language struct {
	Code string
	// Vazio para o ingles, que fica na raiz. Quem chega sem escolher nada cai no
	// texto que vale.
	Directory string
	Suffix    string
	Source    bool
	Text      chrome
}

// O texto da moldura vive junto, e nao espalhado pelo gerador: e ele que muda
// quando a pagina muda de lingua, e uma frase perdida dentro de uma funcao e
// uma frase que ninguem acha na hora de traduzir.
type chrome struct {
	Sections      map[string]string
	Search        string
	SearchLabel   string
	SearchHint    string
	Placeholder   string
	OnThisPage    string
	Pages         string
	Previous      string
	Next          string
	EditThisPage  string
	GeneratedFrom string
	Repository    string
	License       string
	OtherLanguage string
	Theme         string
	Copy          string
	Copied        string
	CopyByHand    string
	LightTheme    string
	DarkTheme     string
	UseLightTheme string
	UseDarkTheme  string
	TypeToSearch  string
	NothingFound  string
	AnchorLabel   string

	// A palavra que o autor escreve no markdown do aviso, e a classe que ela
	// vira. Sem a tabela a folha precisaria de um seletor por lingua para
	// pintar a mesma tarja.
	CalloutClasses map[string]string

	CommandColumns string
	StaleLabel     string
	StaleNotice    string

	ReferenceTitle      string
	ReferenceSummary    string
	ReferenceIntro      string
	ReferenceTop        string
	ReferenceDefault    string
	ReferenceWhole      string
	ReferenceWholeIntro string
	ReferenceColumns    string
	ReferenceRequired   [2]string
	ReferenceTypes      map[string]string
	ReferenceListOf     string
	ReferenceEitherOr   string
	ReferenceShort      string
	ReferenceObject     string

	DecisionsTitle   string
	DecisionsSummary string
	DecisionsIntro   string
	DecisionsColumns string
}

var english = Language{
	Code: "en", Directory: "", Suffix: ".en.md", Source: true,
	Text: chrome{
		Sections:      map[string]string{"start": "Start", "guides": "Guides", "reference": "Reference"},
		Search:        "search",
		SearchLabel:   "Search the documentation",
		SearchHint:    "move · <kbd>enter</kbd> opens · <kbd>esc</kbd> closes",
		Placeholder:   "search the documentation",
		OnThisPage:    "On this page",
		Pages:         "Pages",
		Previous:      "previous",
		Next:          "next",
		EditThisPage:  "edit this page",
		GeneratedFrom: "generated from",
		Repository:    "repository",
		License:       "MIT license",
		OtherLanguage: "Português",
		Theme:         "theme",
		Copy:          "copy",
		Copied:        "copied",
		CopyByHand:    "copy by hand",
		LightTheme:    "light",
		DarkTheme:     "dark",
		UseLightTheme: "Use the light theme",
		UseDarkTheme:  "Use the dark theme",
		TypeToSearch:  "Type to search the {pages} pages.",
		NothingFound:  "Nothing found for “{term}”.",
		AnchorLabel:   "link to this section",
		CalloutClasses: map[string]string{
			"Note": "note", "Warning": "warning", "Important": "important", "Tip": "tip",
		},
		CommandColumns: "| Command | What for |",
		StaleLabel:     "Translation behind",
		StaleNotice: "This page was translated from an older version of the English text. " +
			"Until it catches up, the English page is the one that holds.",

		ReferenceTitle:   "Scenario reference",
		ReferenceSummary: "Every key of the scenario file, generated from the schema.",
		ReferenceIntro: "This page is generated from `docs/braunrate.schema.json`, the same file your " +
			"editor uses to complete the keys. A key braunrate accepts and does not show up here fails the build.",
		ReferenceTop:        "Top of the file",
		ReferenceDefault:    "default",
		ReferenceWhole:      "A whole scenario",
		ReferenceWholeIntro: "Every key below appears in this file. It runs against the built-in target, and a test loads it and validates it on every build.",
		ReferenceColumns:    "| key | type | required | what it does | example |",
		ReferenceRequired:   [2]string{"yes", "no"},
		ReferenceTypes: map[string]string{
			"string": "text", "integer": "integer", "number": "number",
			"boolean": "true or false", "object": "object", "array": "list",
		},
		ReferenceListOf:   "list of",
		ReferenceEitherOr: " or ",
		ReferenceShort:    "short form or object",
		ReferenceObject:   "object",
		DecisionsTitle:    "Decisions",
		DecisionsSummary:  "The recorded architecture decisions, one line each.",
		DecisionsIntro: "Each one records what was decided, what was refused and the criterion that " +
			"reopens the discussion. The full files are in `docs/adr` in the repository, written in Portuguese.",
		DecisionsColumns: "| # | decision |",
	},
}

var brazilianPortuguese = Language{
	Code: "pt-BR", Directory: "pt-BR", Suffix: ".pt-BR.md",
	Text: chrome{
		Sections:      map[string]string{"start": "Começar", "guides": "Guias", "reference": "Referência"},
		Search:        "buscar",
		SearchLabel:   "Buscar na documentação",
		SearchHint:    "navega · <kbd>enter</kbd> abre · <kbd>esc</kbd> fecha",
		Placeholder:   "buscar na documentação",
		OnThisPage:    "Nesta página",
		Pages:         "Páginas",
		Previous:      "anterior",
		Next:          "próxima",
		EditThisPage:  "editar esta página",
		GeneratedFrom: "gerada de",
		Repository:    "repositório",
		License:       "licença MIT",
		OtherLanguage: "English",
		Theme:         "tema",
		Copy:          "copiar",
		Copied:        "copiado",
		CopyByHand:    "copie à mão",
		LightTheme:    "claro",
		DarkTheme:     "escuro",
		UseLightTheme: "Usar tema claro",
		UseDarkTheme:  "Usar tema escuro",
		TypeToSearch:  "Digite para buscar nas {pages} páginas.",
		NothingFound:  "Nada encontrado para “{term}”.",
		AnchorLabel:   "link para esta seção",
		CalloutClasses: map[string]string{
			"Nota": "note", "Atenção": "warning", "Importante": "important", "Dica": "tip",
		},
		CommandColumns: "| Comando | Para quê |",
		StaleLabel:     "Tradução atrasada",
		StaleNotice: "Esta página foi traduzida de uma versão anterior do texto em inglês. " +
			"Até ela alcançar, a página em inglês é a que vale.",

		ReferenceTitle:   "Referência do cenário",
		ReferenceSummary: "Todas as chaves do arquivo de cenário, geradas do schema.",
		ReferenceIntro: "Esta página é gerada de `docs/braunrate.schema.json`, o mesmo arquivo que o seu " +
			"editor usa para completar as chaves. Chave que o braunrate aceita e não aparece aqui reprova o build. " +
			"As descrições saem do schema, que é em inglês desde a 0.6.0.",
		ReferenceTop:        "Topo do arquivo",
		ReferenceDefault:    "padrão",
		ReferenceWhole:      "Um cenário inteiro",
		ReferenceWholeIntro: "Toda chave listada abaixo aparece neste arquivo. Ele roda contra o alvo embutido, e um teste o carrega e o valida a cada build.",
		ReferenceColumns:    "| chave | tipo | obrigatória | o que faz | exemplo |",
		ReferenceRequired:   [2]string{"sim", "não"},
		ReferenceTypes: map[string]string{
			"string": "texto", "integer": "inteiro", "number": "número",
			"boolean": "sim ou não", "object": "objeto", "array": "lista",
		},
		ReferenceListOf:   "lista de",
		ReferenceEitherOr: " ou ",
		ReferenceShort:    "forma curta ou objeto",
		ReferenceObject:   "objeto",
		DecisionsTitle:    "Decisões",
		DecisionsSummary:  "As decisões de arquitetura registradas, uma linha cada.",
		DecisionsIntro: "Cada uma registra o que foi decidido, o que foi recusado e o critério que reabre a " +
			"discussão. Os arquivos completos estão em `docs/adr` no repositório.",
		DecisionsColumns: "| # | decisão |",
	},
}

// O ingles vem primeiro porque ele e a fonte: a build le o original antes de
// conferir o que a traducao diz ter saido dele.
var Languages = []Language{english, brazilianPortuguese}

// De uma pagina para a mesma pagina na outra lingua. O arquivo tem o mesmo nome
// nas duas arvores, entao o que muda e so o caminho ate a raiz — e o seletor
// nunca joga quem clica na pagina inicial.
func (language Language) other() Language {
	for _, candidate := range Languages {
		if candidate.Code != language.Code {
			return candidate
		}
	}
	return language
}

// O que uma pagina desta lingua precisa escrever na frente de um arquivo que
// mora na raiz do site.
func (language Language) toRoot() string {
	if language.Directory == "" {
		return ""
	}
	return "../"
}
