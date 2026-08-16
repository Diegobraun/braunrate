# ADR 0007 — O relatorio declara a variedade observada, nao a declarada

- **Status**: aceito
- **Data**: 2026-08-16
- **Contexto de decisao**: Fase 5, apos o bug de identidade encontrado na Fase 4
- **Relacionados**: [ADR 0003](0003-modelo-de-execucao-e-metrica.md), [ADR 0005](0005-identidade-e-token.md), [ADR 0008](0008-mensageria-e-cadeia-assincrona.md)

## Contexto

Na Fase 4 apareceu um defeito que passou por tres fases sem ser notado: a autenticacao guardava o contexto inteiro da primeira iteracao e o reinjetava nas seguintes, entao **toda execucao autenticada com CSV rodou sobre a primeira linha**. Enquanto isso o relatorio imprimia *"Sementes dos dados: assinantes=1 (mesma semente, mesmos dados)"* — uma frase sobre o que foi **declarado**, que soava como garantia sobre o que **aconteceu**.

A suite nao pegou porque verificava contagem, latencia e taxa de erro. Todos os tres continuam bonitos quando a carga inteira usa o mesmo valor — na verdade ficam **melhores**, porque o alvo responde de cache.

Isso e uma classe de falha, nao um defeito isolado. A mesma forma reaparece em:

- **chave de particao do Kafka** sempre igual: tudo cai numa particao, o resto do cluster fica parado;
- **rota AMQP** unica, quando producao tem varias;
- **cabecalho de tenant** fixo num servico multi-inquilino;
- **corpo identico**, que exercita um caminho de codigo so.

## Decisao

**O relatorio declara a variedade que a execucao produziu, medida, e nao a que o cenario prometeu.**

1. **Um ponto de instrumentacao, nao um por protocolo.** Toda interpolacao passa por `contexto.Resolver`; e la que cada substituicao fica anotada. Com isso caminho, corpo, cabecalho, variavel de GraphQL e chave de mensagem entram de graca, e protocolo novo entra coberto sem escrever nada.
2. **Fatos que so o protocolo conhece entram por `Resposta.Atributos`** — particao do Kafka, rota do AMQP. E a mesma regra do [ADR 0003](0003-modelo-de-execucao-e-metrica.md): instrumentacao no motor, conhecimento no protocolo.
3. **Contagem exata ate 1.024 valores distintos por variavel.** Acima disso o relatorio diz "mais de 1.023". O que importa e distinguir "um valor so" de "muitos"; contar um milhao custaria memoria proporcional a carga sem mudar nenhuma conclusao.
4. **A gravidade separa defeito de escolha:**
   - fonte com varios valores e execucao com um so → **gravidade alta**, resultado invalido, codigo de saida 3. Foi exatamente o caso do bug de identidade.
   - valor fixo declarado no cenario (constante, variavel de ambiente, valor capturado) → **gravidade media**, aviso de leitura: quem escreveu escolheu isso, mas cache por esse valor deixa o numero otimista.
   - fonte que so tem um valor → **sem aviso**. Avisar seria repetir de volta o que a pessoa declarou.
5. **Token de autenticacao e excecao declarada.** Um token para a execucao inteira e o padrao decidido no [ADR 0005](0005-identidade-e-token.md), com limitacao ja impressa no bloco de ambiente. Repetir como aviso grave abafaria os avisos que importam.

## Alternativas descartadas

- **Confiar na declaracao do cenario**: e o que existia, e foi o que escondeu o bug por tres fases.
- **Instrumentar cada protocolo**: protocolo novo nasceria sem cobertura, e a lacuna reapareceria calada.
- **HyperLogLog para contar distintos**: precisao aproximada para uma decisao que so precisa saber se o numero e 1 ou mais que 1.
- **Falhar a execucao imediatamente ao detectar variedade 1**: a execucao ja rodou; o resultado ainda serve para investigar, desde que venha marcado como invalido.

## Consequencias

- O bloco de ambiente perde a frase sobre semente como garantia e ganha linhas como *"3 valores distintos de assinantes.id em 2.375 usos"*.
- Um teste generico executa um cenario que usa dados em caminho, corpo, cabecalho e variavel de GraphQL e falha se algum valor declarado nunca chegar ao alvo. E o teste que faltava.
- Execucao com carga concentrada passa a sair com codigo 3, entao pipeline que rodava verde por engano vai comecar a falhar — isso e o efeito pretendido.
- ~~Fica pendente: variedade por **faixa de valor** (todos os ids diferentes, mas todos do mesmo cliente) e variedade de **corpo** medida por forma, nao por conteudo.~~ **Fechado na Fase 8**, ver adendo abaixo.

## Adendo — faixa de valor e forma de corpo (Fase 8)

A contagem de distintos responde "um valor ou muitos". Ela nao responde **onde** os valores cairam, e essa era a metade que faltava.

6. **Faixa.** Alem de contar, a medicao guarda onde os valores ficaram: intervalo para valores numericos, prefixo comum para texto. `800 valores distintos de pedidos.id em 800 usos, todos comecando com "CLI-A-"` diz o que `800 valores distintos` escondia — todos do mesmo cliente. O intervalo e mantido para **todos** os usos, nao so para os distintos abaixo do teto de 1.024: o teto joga fora exatamente a informacao de onde a carga chegou.
   O prefixo so e declarado a partir de 4 caracteres. Dois ids que comecam com o mesmo digito nao compartilham nada que valha uma frase, e dizer isso soterraria os casos que importam.

7. **Forma de corpo.** O corpo entra na variedade pela **forma** — os campos que carrega e o tipo de cada um — e nao pelo conteudo. Contar corpos distintos contaria ids distintos, que e o que a variedade por valor ja faz. O que o alvo ramifica e a forma: campo a mais, campo ausente, campo que chegou vazio, lista sem itens. Tamanho de lista **nao** e forma; lista vazia e.
   - **Forma unica nao gera aviso nem linha no relatorio.** E o caso normal, vem do que o cenario declarou, e repeti-lo por passo soterraria as linhas que dizem alguma coisa — a mesma regra do item 4 para fonte com um valor so.
   - **Campo vazio gera aviso de gravidade media.** Existe cenario que quer mandar campo em branco, e a ferramenta nao distingue esse de uma interpolacao que resolveu para nada. O que ela pode fazer e dizer que aconteceu, que e o que faltava quando um corpo saia com valor vazio e todo numero do relatorio continuava saudavel.

O ponto de instrumentacao continua sendo um so: a forma sai da configuracao **ja resolvida**, imediatamente antes de ir para o protocolo. Tirar do passo declarado mediria o gabarito e anunciaria uma variedade que a substituicao pode nao ter produzido. Protocolo que carrega corpo declara `RequestBody()`; quem nao carrega nao paga nada.
