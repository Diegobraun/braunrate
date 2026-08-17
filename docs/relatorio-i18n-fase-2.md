# Relatório da Fase 2 — Site bilíngue

Uma fonte, duas saídas. O guia em inglês é o texto que vale; o em português
declara de qual versão dele saiu, e a build avisa quando ele fica para trás.

- **Commits**: 8, de `c7fd46f` a `831a238`, todos com `go test ./...` e
  `golangci-lint run ./...` verdes.
- **Páginas**: 10 por língua, 20 HTML no total, mais uma folha, um script e um
  índice de busca por língua. 684 KB, nada buscado da rede.
- **Defeitos encontrados no caminho**: 5, cada um em commit próprio.
- **Decisão registrada**: [ADR 0020](adr/0020-site-bilingue.md), e as decisões 9
  a 12 em [decisoes-i18n.md](decisoes-i18n.md).

---

## 1. A estrutura

```
docs/guides/
  00-start-introduction.en.md      00-start-introduction.pt-BR.md
  10-start-installation.en.md      10-start-installation.pt-BR.md
  20-start-first-15-minutes.en.md  20-start-first-15-minutes.pt-BR.md
  30-guides-concepts.en.md         30-guides-concepts.pt-BR.md
  40-guides-protocols.en.md        40-guides-protocols.pt-BR.md
  50-guides-recipes.en.md          50-guides-recipes.pt-BR.md
  60-guides-commands.en.md         60-guides-commands.pt-BR.md
  70-guides-troubleshooting.en.md  70-guides-troubleshooting.pt-BR.md
```

O sufixo decide a língua. O resto do nome é o mesmo dos dois lados, e é daí que
sai o seletor sem tabela de equivalência entre endereços:

```
site/index.html              site/pt-BR/index.html
site/concepts.html           site/pt-BR/concepts.html
site/style.css               (uma folha, as duas árvores)
site/page.js                 (um script, as duas árvores)
site/search-index.js         site/pt-BR/search-index.js
```

O número da frente decide a ordem e o meio decide a seção — `start`, `guides`,
`reference` —, e o nome que aparece no menu sai da tabela da língua. As duas
páginas geradas, a referência do cenário e o índice de decisões, entram na
mesma lista.

## 2. Como a tradução declara de onde saiu

Cada guia em português abre com o cabeçalho:

```
---
translated_from: 30-guides-concepts.en.md
source_hash: fb5f0a39ecbc
---
```

O hash são os doze primeiros caracteres do SHA-256 do arquivo em inglês. A build
recalcula e compara. Com o hash certo, ela não diz nada. Com o hash de uma versão
anterior:

```
$ go run ./cmd/site -out site
warning: 30-guides-concepts.pt-BR.md was translated from an older 30-guides-concepts.en.md:
run the translation again and update source_hash
site at site
```

E a página em português abre com a tarja, antes do primeiro parágrafo:

> **Tradução atrasada**
> Esta página foi traduzida de uma versão anterior do texto em inglês. Até ela
> alcançar, a página em inglês é a que vale.

A página em inglês não recebe tarja nenhuma: ela é a fonte, e não tem como estar
atrasada em relação a si mesma.

**A build não reprova.** Reprovar transformaria toda edição no original em uma
edição obrigatória nas duas línguas, e o efeito real disso seria parar de editar
o original. O que reprova é outra coisa: um teste exige que **hoje** nenhuma
tradução publicada esteja atrás da fonte, e outro prova que o mecanismo dispara,
quebrando o hash numa cópia da documentação e conferindo o aviso e a tarja.

```go
func TestNoPublishedTranslationIsBehindItsSource(t *testing.T)
func TestATranslationBehindItsSourceWarnsAndSaysSoOnThePage(t *testing.T)
```

Um cabeçalho ausente ou incompleto não passa em silêncio:

```
30-guides-concepts.pt-BR.md has no front matter: a translation declares
translated_from and source_hash
```

## 3. O seletor, o hreflang e a busca

O seletor troca de língua **sem trocar de página**. Como o arquivo tem o mesmo
nome nas duas árvores, o link é o caminho relativo:

```html
<!-- em /concepts.html -->
<a class="language" href="pt-BR/concepts.html" hreflang="pt-BR" lang="pt-BR">Português</a>

<!-- em /pt-BR/concepts.html -->
<a class="language" href="../concepts.html" hreflang="en" lang="en">English</a>
```

Cada página declara as duas versões para o buscador, com o inglês como padrão:

```html
<link rel="alternate" hreflang="en" href="https://diegobraun.github.io/braunrate/concepts.html">
<link rel="alternate" hreflang="pt-BR" href="https://diegobraun.github.io/braunrate/pt-BR/concepts.html">
<link rel="alternate" hreflang="x-default" href="https://diegobraun.github.io/braunrate/concepts.html">
```

Na página inicial o endereço anunciado é o diretório, e não `index.html`:
anunciar os dois daria ao buscador dois endereços para a mesma página.

**Um índice de busca por língua**, 112 entradas cada. Um índice único devolveria
trecho em português a quem lê em inglês, e o resultado levaria para uma página
que essa pessoa não consegue ler.

## 4. A moldura, e o único catálogo de mensagens do projeto

A Fase 1 recusou catálogo de mensagens: lá a língua seria configuração de quem
executa a ferramenta, e a frase se afastaria do lugar que decide imprimi-la.
Aqui a língua **é o conteúdo da página**, e uma página em português com o menu em
inglês seria metade traduzida. Então a moldura vive numa tabela por língua, em
`internal/site/language.go`:

| O que a tabela carrega | Inglês | Português |
|---|---|---|
| seções do menu | Start, Guides, Reference | Começar, Guias, Referência |
| navegação | previous, next, On this page | anterior, próxima, Nesta página |
| rodapé | edit this page, MIT license | editar esta página, licença MIT |
| busca | search, Nothing found for “{term}”. | buscar, Nada encontrado para “{term}”. |
| botões da página | copy, copied, copy by hand | copiar, copiado, copie à mão |
| rótulo do aviso no markdown | Note, Warning, Important, Tip | Nota, Atenção, Importante, Dica |
| cabeçalho da tabela de comandos | `\| Command \| What for \|` | `\| Comando \| Para quê \|` |
| páginas geradas | Scenario reference, Decisions | Referência do cenário, Decisões |

O rótulo do aviso é o caso que mais custaria depois: a classe do CSS é a mesma
nas duas línguas (`note-warning`), e o que muda é a palavra que o autor escreve
no markdown. Sem a tabela, a folha precisaria de um seletor por língua para
pintar a mesma tarja.

O script e a folha são um só. O que o script escreve na tela vem do próprio HTML:

```html
window.SITE_TEXT = {"copy":"copiar","copied":"copiado","copyByHand":"copie à mão", …}
```

## 5. As duas páginas geradas

A referência do cenário sai do schema; o índice de decisões sai dos títulos dos
ADRs. As duas existem nas duas línguas, com a moldura traduzida e o conteúdo em
inglês — e cada uma diz isso na própria introdução:

> Esta página é gerada de `docs/braunrate.schema.json`, o mesmo arquivo que o seu
> editor usa para completar as chaves. Chave que o braunrate aceita e não aparece
> aqui reprova o build. **As descrições saem do schema, que é em inglês desde a
> 0.6.0.**

> Cada uma registra o que foi decidido, o que foi recusado e o critério que
> reabre a discussão. Os arquivos completos estão em `docs/adr` no repositório.
> *(em inglês: "…in the repository, written in Portuguese.")*

Traduzir as descrições do schema criaria uma segunda descrição livre para
divergir da que o editor mostra durante a escrita do cenário. Traduzir os ADRs
contraria a tabela de camadas da Fase 1.

## 6. Os cinco defeitos que a caminhada encontrou

Nenhum deles foi encontrado lendo código: todos apareceram rodando o comando
para capturar a saída que o guia publica.

| Defeito | Onde | O que o usuário via |
|---|---|---|
| separador de milhar com ponto | `report/terminal.go` e `metrics/variety.go` | `4.800 requests`, que em inglês se lê 4,8 |
| `60 jornadas iniciadas, 0 completas` | `metrics/sanity.go` | evidência do resultado inválido em português |
| `kafka em kafka.staging:9093` | `scenario/yaml_messaging.go` | a preposição em português no meio da linha inglesa |
| `Abra no navegador:` | `server/server.go` | a última linha do aviso do `braunrate ui` |
| `preflight de CORS` | `recorder/recorder.go` | o motivo do descarte na gravação |

Os três últimos escaparam da varredura da Fase 1 porque a palavra portuguesa era
`em`, `de` ou uma frase inteira sem marca de acento — palavras que existem nas
duas línguas ou frases curtas demais para a lista pegar.

O primeiro escapou porque não é palavra nenhuma: é um caractere de pontuação com
significado diferente por convenção regional. **A varredura não pega isso**, e
não vai pegar; o que pega é rodar e ler.

O segundo era uma palavra que a lista tinha — `jornada` — e que o plural
escondeu. A varredura passou a comparar também sem o `s` final, e isso está no
mesmo commit da correção:

```go
// O plural conta como a mesma palavra: "jornadas" escapou da lista por
// um "s" e chegou na tela de quem le em ingles.
if slices.Contains(portugueseWords, lowered) ||
    slices.Contains(portugueseWords, strings.TrimSuffix(lowered, "s")) {
    return word
}
```

## 7. A varredura passou a cobrir o gerador do site

`internal/site` e `cmd/site` estavam fora da varredura desde a Fase 1, com o
motivo escrito no arquivo: o site era português até esta fase. Agora entram, com
duas exceções, cada uma dizendo por que existe:

| Exceção | Por quê |
|---|---|
| `internal/site/language.go` | a moldura nas duas línguas, uma entrada por língua (ADR 0020) |
| `internal/site/slug.go` | a tabela de acentos é a **entrada** da transliteração que escreve a âncora do título, não texto que alguém lê |

A tabela de acentos morava dentro de `site.go`. Excetuar o arquivo inteiro
silenciaria as 400 linhas ao redor dele, então ela foi para um arquivo próprio —
a exceção cobre 20 linhas, e o que sobra continua conferido.

## 8. O que mais mudou junto

O gerador do site tinha identificador, classe de CSS, nome de arquivo e mensagem
de erro em português, o que o ADR 0010 já não permitia. Tudo passou para inglês
no primeiro commit da fase, sem mudança de comportamento:

| Antes | Depois |
|---|---|
| `estilo.css`, `pagina.js`, `indice.js` | `style.css`, `page.js`, `search-index.js` |
| `dobra.go`, `busca.go`, `comandos.go`, `realce.go`, `contraste.go`, `referencia.go`, `decisoes.go` | `hero.go`, `search.go`, `commands.go`, `highlight.go`, `contrast.go`, `reference.go`, `decisions.go` |
| `--fundo`, `--texto`, `--marca`, `--borda` | `--background`, `--text`, `--brand`, `--border` |
| `.dobra`, `.prova`, `.cartao`, `.paginacao`, `.busca` | `.hero`, `.proof`, `.card`, `.pagination`, `.search` |
| `data-tema="escuro"`, `braunrate-tema` | `data-theme="dark"`, `braunrate-theme` |
| `window.INDICE` | `window.SEARCH_INDEX` |
| ```` ```dobra ```` com `lema:`, `resumo:`, `prova:` | ```` ```hero ```` com `motto:`, `summary:`, `proof:` |
| ```` ```yaml trecho ```` | ```` ```yaml fragment ```` |

Os endereços das páginas mudaram junto, porque o nome do arquivo os decide:
`instalacao.html` virou `installation.html`, `conceitos.html` virou
`concepts.html`, `referencia.html` virou `reference.html`. Link salvo de fora
quebra, e isso está registrado como a parte cara de reverter da decisão 9.

## 9. Critério item por item

| Critério | Estado |
|---|---|
| Uma fonte, duas saídas (`.en.md` / `.pt-BR.md`) | ✅ 8 guias em cada língua |
| `translated_from` e `source_hash` no cabeçalho | ✅ nos oito, conferidos na build |
| Aviso de desatualização na build | ✅ no stderr do `cmd/site` |
| Aviso na própria página | ✅ tarja antes do primeiro parágrafo |
| `hreflang` | ✅ as duas línguas mais `x-default` em toda página |
| Seletor preservando a página atual | ✅ mesmo nome de arquivo nas duas árvores |
| Índice de busca por língua | ✅ 112 entradas cada |
| README curto em inglês | ✅ 139 linhas, com a seção "Language" apontando as duas árvores |
| Nenhuma mudança de comportamento | ✅ 5 defeitos corrigidos em commits próprios |
| CI verde e lint zero a cada commit | ✅ 8 de 8 |
| Decisões registradas | ✅ ADR 0020 e decisões 9 a 12 |
| Continua sem buscar nada da rede | ✅ o teste de rede fechada cobre as duas árvores |

## 10. O que ficou de fora, e por quê

- **Uma terceira língua.** `Languages` é uma lista e a estrutura aceita, mas o
  custo de manutenção cresce por língua e a resposta hoje é a mesma da Fase 1.
- **Detecção da língua do navegador.** Quem cai numa tradução sem pedir não sabe
  que existe um original.
- **Tradução dos ADRs e do schema.** Registrada como decisão 12, com o motivo.
- **`docs/auditoria-fricao.md`, `docs/relatorio-experiencia.md` e os relatórios
  internos.** São registro de um momento, e continuam em português.

O que a Fase 3 leva daqui: a página de referência já existe nas duas línguas e
já é gerada do schema, então os exemplos por chave entram no schema e aparecem
nas duas de uma vez.
