'use strict'

// Servido pelo proprio site: nao ha CDN nem dependencia.

document.querySelectorAll('pre').forEach(function (bloco) {
  var caixa = document.createElement('div')
  caixa.className = 'bloco'
  bloco.parentNode.insertBefore(caixa, bloco)
  caixa.appendChild(bloco)

  var botao = document.createElement('button')
  botao.type = 'button'
  botao.className = 'copiar'
  botao.textContent = 'copiar'
  botao.addEventListener('click', function () { copiar(bloco.innerText, botao) })
  caixa.appendChild(botao)
})

function copiar (texto, botao) {
  var rotulo = botao.textContent
  navigator.clipboard.writeText(texto).then(function () {
    botao.textContent = 'copiado'
    setTimeout(function () { botao.textContent = rotulo }, 1500)
  }, function () {
    botao.textContent = 'copie à mão'
  })
}

document.querySelectorAll('.copiar-comando').forEach(function (botao) {
  botao.addEventListener('click', function () {
    var alvo = document.getElementById(botao.dataset.alvo)
    if (alvo) copiar(alvo.textContent, botao)
  })
})

/* -------------------------------------------------------------------- tema */

var botaoDeTema = document.getElementById('tema')
if (botaoDeTema) {
  var escuroNoSistema = window.matchMedia('(prefers-color-scheme: dark)')
  var mostrarTema = function () {
    var atual = document.documentElement.getAttribute('data-tema')
    var escuro = atual ? atual === 'escuro' : escuroNoSistema.matches
    botaoDeTema.textContent = escuro ? 'claro' : 'escuro'
    botaoDeTema.setAttribute('aria-label', escuro ? 'Usar tema claro' : 'Usar tema escuro')
  }
  botaoDeTema.addEventListener('click', function () {
    var atual = document.documentElement.getAttribute('data-tema')
    var escuro = atual ? atual === 'escuro' : escuroNoSistema.matches
    var proximo = escuro ? 'claro' : 'escuro'
    document.documentElement.setAttribute('data-tema', proximo)
    try { localStorage.setItem('braunrate-tema', proximo) } catch (erro) { /* navegacao privada */ }
    mostrarTema()
  })
  escuroNoSistema.addEventListener('change', mostrarTema)
  mostrarTema()
}

/* ------------------------------------------------------------------- busca */

var busca = document.getElementById('busca')
var termo = document.getElementById('termo')
var resultados = document.getElementById('resultados')
var abrirBusca = document.getElementById('abrir-busca')

if (busca && termo && resultados && window.INDICE) {
  var escolhido = 0
  var achados = []
  var eraFoco = null

  var semAcento = function (texto) {
    return texto.toLowerCase().normalize('NFD').replace(/[̀-ͯ]/g, '')
  }

  var abrir = function () {
    eraFoco = document.activeElement
    busca.hidden = false
    termo.value = ''
    termo.focus()
    mostrar([])
  }
  var fechar = function () {
    busca.hidden = true
    if (eraFoco && eraFoco.focus) eraFoco.focus()
  }

  // Pontuacao simples: titulo da secao vale mais que corpo, e todos os termos
  // precisam aparecer. Sem isso "erro 401" traz tudo que fala de erro.
  var procurar = function (consulta) {
    var partes = semAcento(consulta).split(/\s+/).filter(Boolean)
    if (partes.length === 0) return []
    var lista = []
    window.INDICE.forEach(function (entrada) {
      var cabeca = semAcento(entrada.t + ' ' + entrada.s)
      var corpo = semAcento(entrada.x)
      var pontos = 0
      for (var i = 0; i < partes.length; i++) {
        var noCabeca = cabeca.indexOf(partes[i]) >= 0
        var noCorpo = corpo.indexOf(partes[i]) >= 0
        if (!noCabeca && !noCorpo) return
        pontos += noCabeca ? 8 : 0
        pontos += noCorpo ? 1 : 0
      }
      if (semAcento(entrada.s).indexOf(semAcento(consulta)) === 0) pontos += 6
      lista.push({ entrada: entrada, pontos: pontos, trecho: recortar(entrada.x, partes[0]) })
    })
    return lista.sort(function (a, b) { return b.pontos - a.pontos }).slice(0, 12)
  }

  var recortar = function (texto, parte) {
    var posicao = semAcento(texto).indexOf(parte)
    if (posicao < 0) return texto.slice(0, 120)
    var inicio = Math.max(0, posicao - 45)
    return (inicio > 0 ? '…' : '') + texto.slice(inicio, inicio + 150)
  }

  var escapar = function (texto) {
    return texto.replace(/[&<>"]/g, function (caractere) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[caractere]
    })
  }

  var destacar = function (trecho, partes) {
    var texto = escapar(trecho)
    partes.forEach(function (parte) {
      var semAcentoTexto = semAcento(texto)
      var posicao = semAcentoTexto.indexOf(parte)
      if (posicao < 0 || semAcentoTexto.length !== texto.length) return
      texto = texto.slice(0, posicao) + '<mark>' + texto.slice(posicao, posicao + parte.length) +
        '</mark>' + texto.slice(posicao + parte.length)
    })
    return texto
  }

  var mostrar = function (lista) {
    achados = lista
    escolhido = 0
    if (termo.value.trim() === '') {
      resultados.innerHTML = '<li class="vazio">Digite para buscar nas ' +
        (new Set(window.INDICE.map(function (e) { return e.p })).size) + ' páginas.</li>'
      return
    }
    if (lista.length === 0) {
      resultados.innerHTML = '<li class="vazio">Nada encontrado para “' + escapar(termo.value) + '”.</li>'
      return
    }
    var partes = semAcento(termo.value).split(/\s+/).filter(Boolean)
    resultados.innerHTML = lista.map(function (achado, posicao) {
      var entrada = achado.entrada
      var destino = entrada.p + (entrada.a ? '#' + entrada.a : '')
      return '<li' + (posicao === 0 ? ' aria-selected="true"' : '') + '><a href="' + escapar(destino) + '">' +
        '<span class="onde">' + escapar(entrada.t) + '</span>' +
        '<p class="titulo">' + escapar(entrada.s) + '</p>' +
        '<p class="trecho">' + destacar(achado.trecho, partes) + '</p></a></li>'
    }).join('')
  }

  var mover = function (passo) {
    if (achados.length === 0) return
    var itens = resultados.querySelectorAll('li')
    itens[escolhido].removeAttribute('aria-selected')
    escolhido = (escolhido + passo + itens.length) % itens.length
    itens[escolhido].setAttribute('aria-selected', 'true')
    itens[escolhido].scrollIntoView({ block: 'nearest' })
  }

  termo.addEventListener('input', function () { mostrar(procurar(termo.value)) })

  if (abrirBusca) abrirBusca.addEventListener('click', abrir)

  document.addEventListener('keydown', function (evento) {
    var digitando = /^(INPUT|TEXTAREA|SELECT)$/.test(document.activeElement.tagName)
    if (busca.hidden && !digitando && (evento.key === '/' || ((evento.metaKey || evento.ctrlKey) && evento.key === 'k'))) {
      evento.preventDefault()
      abrir()
      return
    }
    if (busca.hidden) return
    if (evento.key === 'Escape') { evento.preventDefault(); fechar() }
    if (evento.key === 'ArrowDown') { evento.preventDefault(); mover(1) }
    if (evento.key === 'ArrowUp') { evento.preventDefault(); mover(-1) }
    if (evento.key === 'Enter') {
      var escolha = resultados.querySelector('li[aria-selected="true"] a')
      if (escolha) { evento.preventDefault(); window.location.href = escolha.getAttribute('href') }
    }
    if (evento.key === 'Tab') {
      evento.preventDefault()
      termo.focus()
    }
  })

  busca.addEventListener('click', function (evento) { if (evento.target === busca) fechar() })
}

/* ---------------------------------------------------------- indice lateral */

var indice = document.querySelector('.indice')
if (indice && 'IntersectionObserver' in window) {
  var links = {}
  indice.querySelectorAll('a').forEach(function (link) {
    links[link.getAttribute('href').slice(1)] = link
  })
  var observador = new IntersectionObserver(function (entradas) {
    entradas.forEach(function (entrada) {
      var link = links[entrada.target.id]
      if (link && entrada.isIntersecting) {
        indice.querySelectorAll('a').forEach(function (outro) { outro.removeAttribute('aria-current') })
        link.setAttribute('aria-current', 'true')
      }
    })
  }, { rootMargin: '0px 0px -75% 0px' })
  document.querySelectorAll('article h2[id], article h3[id]').forEach(function (titulo) {
    observador.observe(titulo)
  })
}
