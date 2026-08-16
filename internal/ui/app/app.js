'use strict'

// A tela lê e grava o arquivo .yaml de -dir, e nada mais. Não existe campo que
// viva só aqui: o que a interface sabe fazer, ela faz editando o arquivo, e o
// comando equivalente do terminal fica visível o tempo todo.

const estado = { cenarios: [], execucoes: [], diretorio: '.', seguindo: null }

const conteudo = document.getElementById('conteudo')
const listaCenarios = document.getElementById('cenarios')
const listaExecucoes = document.getElementById('execucoes')
const comando = document.getElementById('comando')

function escapar (texto) {
  return String(texto ?? '').replace(/[&<>"']/g, function (caractere) {
    return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[caractere]
  })
}

function mostrarComando (texto) {
  comando.textContent = texto
}

document.getElementById('explicacoes').addEventListener('change', function () {
  document.body.classList.toggle('sem-explicacoes', !this.checked)
})

document.getElementById('copiar').addEventListener('click', async function () {
  try {
    await navigator.clipboard.writeText(comando.textContent)
    this.textContent = 'copiado'
    setTimeout(() => { this.textContent = 'copiar' }, 1500)
  } catch (erro) {
    this.textContent = 'copie à mão'
  }
})

async function pedir (caminho, opcoes) {
  const resposta = await fetch(caminho, opcoes)
  const texto = await resposta.text()
  let corpo = texto
  if ((resposta.headers.get('content-type') || '').includes('json')) {
    try { corpo = JSON.parse(texto) } catch (erro) { corpo = { message: texto } }
  }
  return { ok: resposta.ok, status: resposta.status, corpo }
}

function mensagemDeErro (resposta) {
  if (resposta.corpo && resposta.corpo.message) return resposta.corpo.message
  if (typeof resposta.corpo === 'string' && resposta.corpo) return resposta.corpo
  return 'não consegui falar com o braunrate; ele ainda está no ar?'
}

// A barra lateral é o estado do diretório, então ela é redesenhada sempre que
// algo muda no disco ou uma execução começa.
async function recarregarLaterais () {
  const cenarios = await pedir('/scenarios')
  if (cenarios.ok) estado.cenarios = cenarios.corpo.scenarios || []
  const execucoes = await pedir('/runs')
  if (execucoes.ok) estado.execucoes = execucoes.corpo.runs || []
  desenharLaterais()
}

function desenharLaterais () {
  const atual = decodeURIComponent((location.hash.split('/')[2] || ''))
  if (estado.cenarios.length === 0) {
    listaCenarios.innerHTML = '<li class="vazio">nenhum nesta pasta</li>'
  } else {
    listaCenarios.innerHTML = estado.cenarios.map(function (cenario) {
      const marca = cenario.name === atual ? ' aria-current="page"' : ''
      return `<li><a class="item" href="#/cenario/${encodeURIComponent(cenario.name)}"${marca}>${escapar(cenario.name)}</a></li>`
    }).join('')
  }

  if (estado.execucoes.length === 0) {
    listaExecucoes.innerHTML = '<li class="vazio">nenhuma ainda</li>'
  } else {
    listaExecucoes.innerHTML = estado.execucoes.slice(0, 8).map(function (execucao) {
      return `<li><a class="item" href="#/execucao/${execucao.id}">${escapar(execucao.id)} · ${escapar(execucao.scenario)}
        <span class="estado">${escapar(execucao.verdict || execucao.status)}</span></a></li>`
    }).join('')
  }
}

// ---------------------------------------------------------------- telas

function telaCenarios () {
  mostrarComando(`braunrate ui -dir ${estado.diretorio}`)
  if (estado.cenarios.length === 0) {
    conteudo.innerHTML = `
      <div class="vazio-tela">
        <h1>Nenhum cenário nesta pasta ainda</h1>
        <p class="legenda">A interface edita os arquivos <code>.yaml</code> de <code>${escapar(estado.diretorio)}</code>.
          Três formas de ter o primeiro:</p>
        <div class="opcoes">
          <a class="opcao" href="#/novo"><b>Começar do zero</b>
            <span>um formulário curto, que grava um cenário comentado</span></a>
          <a class="opcao" href="#/importar"><b>Importar um cURL</b>
            <span>cole o comando copiado do painel de rede do navegador</span></a>
          <a class="opcao" href="#/demonstracao"><b>Ver a demonstração</b>
            <span>roda um exemplo completo, sem configurar nada</span></a>
        </div>
      </div>`
    return
  }
  conteudo.innerHTML = `
    <h1>Cenários</h1>
    <p class="legenda">${estado.cenarios.length} arquivo(s) em <code>${escapar(estado.diretorio)}</code>.
      Abrir um cenário abre o próprio arquivo: o que você grava aqui é o que o terminal lê.</p>
    <table>
      <tr><th>arquivo</th><th>caminho</th></tr>
      ${estado.cenarios.map(function (cenario) {
        return `<tr><td><a href="#/cenario/${encodeURIComponent(cenario.name)}">${escapar(cenario.name)}</a></td>
          <td><code>${escapar(cenario.path)}</code></td></tr>`
      }).join('')}
    </table>`
}

function telaDemonstracao () {
  mostrarComando('braunrate demo')
  conteudo.innerHTML = `
    <h1>A demonstração roda no terminal</h1>
    <p class="legenda">Ela sobe um alvo de mentira, escreve o cenário que vai rodar e explica cada
      número enquanto eles aparecem. Não dá para fazer isso aqui dentro sem esconder metade do que
      ela ensina, então o comando é este:</p>
    <pre>braunrate demo</pre>
    <p>E, para ver a ferramenta pegando um problema de verdade:</p>
    <pre>braunrate demo --com-falha</pre>
    <p><a class="botao" href="#/">voltar</a></p>`
}

function telaImportar () {
  mostrarComando('braunrate import curl "<comando>" -output cenario.yaml')
  conteudo.innerHTML = `
    <h1>Importar um cURL</h1>
    <p class="legenda">A importação acontece no terminal, e o arquivo que ela grava aparece aqui na
      lista assim que existir. Copie a requisição no painel de rede do navegador com
      "Copiar como cURL" e rode:</p>
    <pre>braunrate import curl "&lt;cole aqui&gt;" -output cenario.yaml</pre>
    <p>O token vira <code>\${TOKEN}</code>, lido do ambiente, e não vai para o repositório.</p>
    <p><a class="botao" href="#/">voltar</a></p>`
}

const modeloDeCenario = ({ nome, alvo, metodo, caminho, taxa, duracao, p95, erros }) =>
`# yaml-language-server: $schema=https://raw.githubusercontent.com/Diegobraun/braunrate/main/docs/braunrate.schema.json
nome: ${nome}
alvo: ${alvo}

# taxa: quantas requisições por segundo o braunrate dispara. Ele dispara nesse
# ritmo esteja o alvo rápido ou lento, que é o que usuários de verdade fazem.
carga:
  perfis:
    - patamar: { taxa: ${taxa}/s, durante: ${duracao} }

cenario:
  - http: ${metodo} ${caminho}
    nome: ${nome.toLowerCase()}
    verificar: { status: 200 }

# critério de aceite: se estourar, a execução sai com código 1 e o seu CI reprova.
slo:
  - global: { p95: < ${p95}, erros: < ${erros} }
`

function telaNovo () {
  mostrarComando('braunrate new cenario.yaml')
  conteudo.innerHTML = `
    <h1>Começar do zero</h1>
    <p class="legenda">O formulário grava um arquivo <code>.yaml</code> comentado. Daqui em diante o
      arquivo é a verdade: tudo o que ele não cobre se edita no texto, aqui mesmo.</p>
    <form class="formulario" id="formulario">
      <label><span>Nome do arquivo</span><input name="arquivo" value="cenario.yaml" required>
        <div class="ajuda">Fica em <code>${escapar(estado.diretorio)}</code>.</div></label>
      <label><span>Nome do cenário</span><input name="nome" value="Consulta de pedidos" required>
        <div class="ajuda">Aparece no topo do relatório.</div></label>
      <label><span>Alvo</span><input name="alvo" value="http://127.0.0.1:8080" required>
        <div class="ajuda">O serviço a medir. Sem serviço à mão, <code>braunrate target</code> sobe um.</div></label>
      <div class="dupla">
        <label><span>Método</span>
          <select name="metodo"><option>GET</option><option>POST</option><option>PUT</option><option>PATCH</option><option>DELETE</option></select></label>
        <label><span>Caminho</span><input name="caminho" value="/pedidos/1" required>
          <div class="ensina">Caminho com valor fixo mede o cache do alvo, não o alvo: toda requisição
            vai ser idêntica. Depois de gravar, troque <code>/pedidos/1</code> por
            <code>/pedidos/${'$'}{pedidos.id}</code> e declare de onde <code>${'$'}{pedidos.id}</code> vem.</div></label>
      </div>
      <div class="dupla">
        <label><span>Taxa (requisições por segundo)</span><input name="taxa" value="100" required>
          <div class="ensina">Taxa é o ritmo em que o gerador dispara, esteja o alvo rápido ou lento —
            como usuários de verdade fazem. Ferramenta que espera a resposta anterior alivia o sistema
            justamente quando ele está sofrendo.</div></label>
        <label><span>Duração</span><input name="duracao" value="30s" required></label>
      </div>
      <div class="dupla">
        <label><span>95% das respostas em até</span><input name="p95" value="500ms" required>
          <div class="ensina">"95% em até 500ms" quer dizer que 5% das pessoas esperaram mais que isso.
            Média não entra no relatório porque esconde justamente essas.</div></label>
        <label><span>Taxa de erro máxima (%)</span><input name="erros" value="0.1" required>
          <div class="ensina">Estes dois limites são o critério de aceite: se algum estourar, o braunrate
            sai com código 1 e a esteira reprova o build.</div></label>
      </div>
      <div class="barra"><button class="principal" type="submit">Gravar cenário</button>
        <a class="botao" href="#/">cancelar</a></div>
      <div id="resultado"></div>
    </form>`

  document.getElementById('formulario').addEventListener('submit', async function (evento) {
    evento.preventDefault()
    const campos = Object.fromEntries(new FormData(this).entries())
    let arquivo = campos.arquivo.trim()
    if (!arquivo.endsWith('.yaml') && !arquivo.endsWith('.yml')) arquivo += '.yaml'

    const resposta = await pedir(`/scenarios/${encodeURIComponent(arquivo)}/text`,
      { method: 'PUT', body: modeloDeCenario(campos) })
    const resultado = document.getElementById('resultado')
    if (!resposta.ok) {
      resultado.innerHTML = `<div class="aviso erro"><h3>Não gravei</h3><p>${escapar(mensagemDeErro(resposta))}</p></div>`
      return
    }
    await recarregarLaterais()
    location.hash = `#/cenario/${encodeURIComponent(arquivo)}`
  })
}

function telaEditor (nome) {
  mostrarComando(`braunrate validate ${nome}`)
  conteudo.innerHTML = `
    <h1>${escapar(nome)}</h1>
    <p class="legenda">Este é o arquivo, com os comentários que você escreveu. Editar por fora é
      enxergado aqui: recarregue a página.</p>
    <div class="barra">
      <button id="salvar">Salvar</button>
      <button id="depurar">Depurar uma iteração</button>
      <button id="executar" class="principal">Executar com carga</button>
      <span class="direita" id="situacao">carregando…</span>
    </div>
    <div id="veredito"></div>
    <textarea id="texto" spellcheck="false" aria-label="cenário em YAML"></textarea>
    <div id="saida"></div>`

  const texto = document.getElementById('texto')
  const situacao = document.getElementById('situacao')
  const veredito = document.getElementById('veredito')
  const saida = document.getElementById('saida')

  pedir(`/scenarios/${encodeURIComponent(nome)}/text`).then(function (resposta) {
    if (!resposta.ok) {
      situacao.textContent = ''
      veredito.innerHTML = `<div class="aviso erro"><h3>Não consegui abrir</h3><p>${escapar(mensagemDeErro(resposta))}</p></div>`
      return
    }
    texto.value = resposta.corpo
    situacao.textContent = 'gravado'
    validar()
  })

  let atraso = null
  texto.addEventListener('input', function () {
    situacao.textContent = 'não gravado'
    clearTimeout(atraso)
    atraso = setTimeout(validar, 700)
  })

  // A validação confere o rascunho da tela, não o arquivo gravado, e pelo mesmo
  // caminho de leitura do terminal: o editor nunca aprova o que o comando
  // reprovaria.
  async function validar () {
    const resposta = await pedir(`/scenarios/${encodeURIComponent(nome)}/validate`,
      { method: 'POST', body: texto.value })
    if (resposta.ok) {
      const linhas = (resposta.corpo.lines || []).map(escapar).join('\n')
      veredito.innerHTML = `<div class="aviso ok"><h3>Cenário válido</h3><pre>${linhas}</pre></div>`
      return
    }
    const posicao = resposta.corpo.line ? ` (linha ${resposta.corpo.line}, coluna ${resposta.corpo.column})` : ''
    veredito.innerHTML = `<div class="aviso erro"><h3>O cenário não fecha${escapar(posicao)}</h3>
      <pre>${escapar(mensagemDeErro(resposta))}</pre></div>`
  }

  async function salvar () {
    situacao.textContent = 'gravando…'
    const resposta = await pedir(`/scenarios/${encodeURIComponent(nome)}/text`,
      { method: 'PUT', body: texto.value })
    if (!resposta.ok) {
      situacao.textContent = 'não gravou'
      veredito.innerHTML = `<div class="aviso erro"><h3>Não gravei</h3><p>${escapar(mensagemDeErro(resposta))}</p></div>`
      return false
    }
    situacao.textContent = 'gravado'
    return true
  }

  document.getElementById('salvar').addEventListener('click', salvar)

  document.getElementById('depurar').addEventListener('click', async function () {
    mostrarComando(`braunrate debug ${nome}`)
    if (!await salvar()) return
    saida.innerHTML = '<p class="carregando">rodando uma iteração…</p>'
    const resposta = await pedir(`/scenarios/${encodeURIComponent(nome)}/debug`, { method: 'POST' })
    if (!resposta.ok) {
      saida.innerHTML = `<div class="aviso erro"><h3>A depuração parou</h3><pre>${escapar(mensagemDeErro(resposta))}</pre></div>`
      return
    }
    const classe = resposta.corpo.complete ? 'ok' : 'erro'
    const titulo = resposta.corpo.complete
      ? 'Iteração completa: dá para rodar com carga'
      : 'A iteração não chegou ao fim; a carga só vale depois que ela passa'
    saida.innerHTML = `<h2>Depuração</h2><div class="aviso ${classe}"><h3>${titulo}</h3></div>
      <pre>${escapar(resposta.corpo.text)}</pre>`
  })

  document.getElementById('executar').addEventListener('click', async function () {
    mostrarComando(`braunrate execute ${nome}`)
    if (!await salvar()) return
    saida.innerHTML = '<p class="carregando">começando…</p>'
    const resposta = await pedir(`/scenarios/${encodeURIComponent(nome)}/runs`, { method: 'POST' })
    if (!resposta.ok) {
      saida.innerHTML = `<div class="aviso erro"><h3>Não comecei</h3><pre>${escapar(mensagemDeErro(resposta))}</pre></div>`
      return
    }
    await recarregarLaterais()
    location.hash = `#/execucao/${resposta.corpo.id}`
  })
}

function telaExecucoes () {
  mostrarComando('braunrate compare antes.json depois.json')
  if (estado.execucoes.length === 0) {
    conteudo.innerHTML = `
      <div class="vazio-tela">
        <h1>Nenhuma execução ainda</h1>
        <p class="legenda">As execuções vivem na memória deste processo e somem quando ele reinicia —
          não há banco, porque o banco seria uma segunda verdade ao lado dos arquivos.</p>
        <p><a class="botao" href="#/">escolher um cenário</a></p>
      </div>`
    return
  }
  conteudo.innerHTML = `
    <h1>Execuções</h1>
    <p class="legenda">Marque duas para comparar. Comparar exige as duas terminadas.</p>
    <div class="barra"><button id="comparar" disabled>Comparar as duas marcadas</button>
      <span class="direita" id="marcadas">nenhuma marcada</span></div>
    <table>
      <tr><th></th><th>id</th><th>cenário</th><th>veredito</th><th>quando</th></tr>
      ${estado.execucoes.map(function (execucao) {
        return `<tr class="selecionavel" data-id="${escapar(execucao.id)}">
          <td><input type="checkbox" style="width:auto" data-marca="${escapar(execucao.id)}"></td>
          <td><a href="#/execucao/${escapar(execucao.id)}">${escapar(execucao.id)}</a></td>
          <td>${escapar(execucao.scenario)}</td>
          <td>${escapar(execucao.verdict || execucao.status)}</td>
          <td>${escapar(new Date(execucao.started_at).toLocaleString('pt-BR'))}</td></tr>`
      }).join('')}
    </table>
    <div id="comparacao"></div>`

  const botao = document.getElementById('comparar')
  const marcadas = document.getElementById('marcadas')
  function selecionadas () {
    return Array.from(document.querySelectorAll('[data-marca]:checked')).map(caixa => caixa.dataset.marca)
  }
  conteudo.addEventListener('change', function () {
    const escolhidas = selecionadas()
    botao.disabled = escolhidas.length !== 2
    marcadas.textContent = escolhidas.length === 0 ? 'nenhuma marcada' : escolhidas.join(' e ')
  })
  botao.addEventListener('click', function () {
    const [primeira, segunda] = selecionadas()
    location.hash = `#/comparacao/${primeira}/${segunda}`
  })
}

async function telaExecucao (id) {
  const execucao = estado.execucoes.find(uma => uma.id === id)
  mostrarComando(`braunrate execute ${execucao ? execucao.scenario : '<cenario>.yaml'} -html relatorio.html`)
  conteudo.innerHTML = `
    <h1>Execução ${escapar(id)}</h1>
    <p class="legenda" id="de-qual-cenario">${execucao ? escapar(execucao.scenario) : 'carregando…'}</p>
    <div id="andamento"></div>
    <div id="relatorio"></div>`

  const andamento = document.getElementById('andamento')
  const relatorio = document.getElementById('relatorio')

  if (execucao && execucao.status === 'running') {
    andamento.innerHTML = `
      <div class="barra"><span class="direita">em andamento — fechar esta página não interrompe nada;
        para interromper, use Ctrl+C no terminal que subiu a interface</span></div>
      <pre id="linhas">esperando a primeira linha…</pre>`
    await seguir(id, document.getElementById('linhas'))
    await recarregarLaterais()
    const terminada = estado.execucoes.find(uma => uma.id === id)
    andamento.querySelector('.direita').textContent = terminada && terminada.verdict
      ? `terminou: ${terminada.verdict}`
      : 'terminou'
  }

  const resposta = await pedir(`/runs/${id}`)
  if (!resposta.ok) {
    relatorio.innerHTML = `<div class="aviso erro"><h3>Sem relatório</h3><pre>${escapar(mensagemDeErro(resposta))}</pre></div>`
    return
  }
  const documento = resposta.corpo
  const doCenario = documento.execucao && documento.execucao.cenario
  document.getElementById('de-qual-cenario').textContent = doCenario || (execucao ? execucao.scenario : '')
  if (documento.status === 'running') {
    relatorio.innerHTML = '<p class="carregando">ainda rodando…</p>'
    return
  }
  // Execução inválida não ganha número em primeiro plano: o motivo vem antes, e
  // nenhum verde aparece acima dele.
  const sanidade = documento.sanidade || {}
  if (sanidade.verificada && !sanidade.valida) {
    relatorio.innerHTML = `<div class="aviso erro"><h3>Resultado inválido</h3>
      <p>${escapar(sanidade.frase)}</p>
      <div class="ensina">Nenhum número desta execução vale como resposta: ela não aprova nem reprova, e
        o código de saída é 3. Corrija o que está abaixo e rode de novo.</div></div>` +
      (sanidade.achados || []).map(achado =>
        `<div class="aviso erro"><p>${escapar(achado.mensagem)}</p><p><small>${escapar(achado.evidencia)}</small></p></div>`).join('')
  }
  relatorio.innerHTML += `<h2>Relatório</h2><iframe src="/runs/${encodeURIComponent(id)}/report" title="relatório"></iframe>`
}

// O stream é o mesmo texto que o terminal imprime, uma linha por atualização.
async function seguir (id, destino) {
  const resposta = await fetch(`/runs/${encodeURIComponent(id)}/stream`)
  if (!resposta.ok || !resposta.body) {
    destino.textContent = 'não consegui acompanhar esta execução'
    return
  }
  const leitor = resposta.body.getReader()
  const decodificador = new TextDecoder()
  let acumulado = ''
  destino.textContent = ''
  for (;;) {
    const { done, value } = await leitor.read()
    if (done) break
    acumulado += decodificador.decode(value, { stream: true })
    destino.textContent = acumulado
    destino.scrollTop = destino.scrollHeight
  }
}

function telaComparacao (antes, depois) {
  mostrarComando(`braunrate compare ${antes}.json ${depois}.json -html comparacao.html`)
  conteudo.innerHTML = `
    <h1>${escapar(antes)} contra ${escapar(depois)}</h1>
    <p class="legenda">Duas execuções não dão intervalo de confiança: variação abaixo de 5% é ruído,
      e ressalva que explique a diferença sozinha tira o veredito.</p>
    <iframe src="/runs/${encodeURIComponent(antes)}/comparison/${encodeURIComponent(depois)}/report" title="comparação"></iframe>
    <p><a class="botao" href="#/execucoes">voltar</a></p>`
}

// ---------------------------------------------------------------- roteamento

async function desenhar () {
  const partes = location.hash.replace(/^#\/?/, '').split('/').map(decodeURIComponent)
  desenharLaterais()
  switch (partes[0]) {
    case 'novo': return telaNovo()
    case 'importar': return telaImportar()
    case 'demonstracao': return telaDemonstracao()
    case 'execucoes': return telaExecucoes()
    case 'cenario': return telaEditor(partes[1])
    case 'execucao': return telaExecucao(partes[1])
    case 'comparacao': return telaComparacao(partes[1], partes[2])
    default: return telaCenarios()
  }
}

window.addEventListener('hashchange', desenhar)

async function comecar () {
  const saude = await pedir('/health')
  if (!saude.ok) {
    conteudo.innerHTML = `<div class="aviso erro"><h3>O braunrate não respondeu</h3>
      <p>A interface é servida pelo próprio comando; se ele parou, suba de novo com
      <code>braunrate ui</code>.</p></div>`
    return
  }
  document.getElementById('versao').textContent = saude.corpo.version
  estado.diretorio = saude.corpo.directory || '.'
  await recarregarLaterais()
  await desenhar()
}

comecar()
