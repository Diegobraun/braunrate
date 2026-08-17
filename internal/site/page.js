'use strict'

// Servido pelo proprio site: nao ha CDN nem dependencia.

// A folha e o script sao os mesmos nas duas linguas; o que muda e esta tabela,
// que a propria pagina declara antes de carregar o script.
var TEXT = window.SITE_TEXT || {}

document.querySelectorAll('pre').forEach(function (block) {
  var box = document.createElement('div')
  box.className = 'block'
  block.parentNode.insertBefore(box, block)
  box.appendChild(block)

  var button = document.createElement('button')
  button.type = 'button'
  button.className = 'copy'
  button.textContent = TEXT.copy
  button.addEventListener('click', function () { copy(block.innerText, button) })
  box.appendChild(button)
})

function copy (text, button) {
  var label = button.textContent
  navigator.clipboard.writeText(text).then(function () {
    button.textContent = TEXT.copied
    setTimeout(function () { button.textContent = label }, 1500)
  }, function () {
    button.textContent = TEXT.copyByHand
  })
}

document.querySelectorAll('.copy-command').forEach(function (button) {
  button.addEventListener('click', function () {
    var target = document.getElementById(button.dataset.target)
    if (target) copy(target.textContent, button)
  })
})

/* --------------------------------------------------------------- referencia */

// A referencia e a unica pagina cujas celulas carregam valor para colar no
// cenario, e ir ate o exemplo com o mouse para selecionar texto de uma celula
// estreita e onde a pessoa desiste e digita de novo.
document.querySelectorAll('article.reference td code').forEach(function (value) {
  value.className = 'copyable'
  value.title = TEXT.copy
  value.addEventListener('click', function () {
    var previous = value.title
    navigator.clipboard.writeText(value.textContent).then(function () {
      value.title = TEXT.copied
      value.classList.add('copied')
      setTimeout(function () {
        value.title = previous
        value.classList.remove('copied')
      }, 1200)
    }, function () {
      value.title = TEXT.copyByHand
    })
  })
})

/* -------------------------------------------------------------------- menu */

// O 'open' vem no HTML para a pagina sem script continuar navegavel, e fechar em
// CSS nao da: 'details' aberto nao volta a fechar por folha de estilo.
var menu = document.getElementById('menu')
if (menu && window.matchMedia('(max-width: 860px)').matches) {
  menu.open = false
}

/* -------------------------------------------------------------------- tema */

var themeButton = document.getElementById('theme')
if (themeButton) {
  var systemDark = window.matchMedia('(prefers-color-scheme: dark)')
  var showTheme = function () {
    var current = document.documentElement.getAttribute('data-theme')
    var dark = current ? current === 'dark' : systemDark.matches
    themeButton.textContent = dark ? TEXT.lightTheme : TEXT.darkTheme
    themeButton.setAttribute('aria-label', dark ? TEXT.useLightTheme : TEXT.useDarkTheme)
  }
  themeButton.addEventListener('click', function () {
    var current = document.documentElement.getAttribute('data-theme')
    var dark = current ? current === 'dark' : systemDark.matches
    var next = dark ? 'light' : 'dark'
    document.documentElement.setAttribute('data-theme', next)
    try { localStorage.setItem('braunrate-theme', next) } catch (error) { /* navegacao privada */ }
    showTheme()
  })
  systemDark.addEventListener('change', showTheme)
  showTheme()
}

/* ------------------------------------------------------------------- busca */

var search = document.getElementById('search')
var term = document.getElementById('term')
var results = document.getElementById('results')
var openSearch = document.getElementById('open-search')

if (search && term && results && window.SEARCH_INDEX) {
  var chosen = 0
  var found = []
  var hadFocus = null

  var withoutAccents = function (text) {
    return text.toLowerCase().normalize('NFD').replace(/[̀-ͯ]/g, '')
  }

  var open = function () {
    hadFocus = document.activeElement
    search.hidden = false
    term.value = ''
    term.focus()
    show([])
  }
  var close = function () {
    search.hidden = true
    if (hadFocus && hadFocus.focus) hadFocus.focus()
  }

  // Pontuacao simples: titulo da secao vale mais que corpo, e todos os termos
  // precisam aparecer. Sem isso "erro 401" traz tudo que fala de erro.
  var look = function (query) {
    var parts = withoutAccents(query).split(/\s+/).filter(Boolean)
    if (parts.length === 0) return []
    var list = []
    window.SEARCH_INDEX.forEach(function (entry) {
      var head = withoutAccents(entry.t + ' ' + entry.s)
      var body = withoutAccents(entry.x)
      var points = 0
      for (var i = 0; i < parts.length; i++) {
        var inHead = head.indexOf(parts[i]) >= 0
        var inBody = body.indexOf(parts[i]) >= 0
        if (!inHead && !inBody) return
        points += inHead ? 8 : 0
        points += inBody ? 1 : 0
      }
      if (withoutAccents(entry.s).indexOf(withoutAccents(query)) === 0) points += 6
      list.push({ entry: entry, points: points, excerpt: cut(entry.x, parts[0]) })
    })
    return list.sort(function (a, b) { return b.points - a.points }).slice(0, 12)
  }

  var cut = function (text, part) {
    var position = withoutAccents(text).indexOf(part)
    if (position < 0) return text.slice(0, 120)
    var start = Math.max(0, position - 45)
    return (start > 0 ? '…' : '') + text.slice(start, start + 150)
  }

  var escape = function (text) {
    return text.replace(/[&<>"]/g, function (character) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[character]
    })
  }

  var highlight = function (excerpt, parts) {
    var text = escape(excerpt)
    parts.forEach(function (part) {
      var plain = withoutAccents(text)
      var position = plain.indexOf(part)
      if (position < 0 || plain.length !== text.length) return
      text = text.slice(0, position) + '<mark>' + text.slice(position, position + part.length) +
        '</mark>' + text.slice(position + part.length)
    })
    return text
  }

  var show = function (list) {
    found = list
    chosen = 0
    if (term.value.trim() === '') {
      results.innerHTML = '<li class="empty">' +
        TEXT.typeToSearch.replace('{pages}', new Set(window.SEARCH_INDEX.map(function (e) { return e.p })).size) +
        '</li>'
      return
    }
    if (list.length === 0) {
      results.innerHTML = '<li class="empty">' +
        TEXT.nothingFound.replace('{term}', escape(term.value)) + '</li>'
      return
    }
    var parts = withoutAccents(term.value).split(/\s+/).filter(Boolean)
    results.innerHTML = list.map(function (hit, position) {
      var entry = hit.entry
      var destination = entry.p + (entry.a ? '#' + entry.a : '')
      return '<li' + (position === 0 ? ' aria-selected="true"' : '') + '><a href="' + escape(destination) + '">' +
        '<span class="where">' + escape(entry.t) + '</span>' +
        '<p class="title">' + escape(entry.s) + '</p>' +
        '<p class="excerpt">' + highlight(hit.excerpt, parts) + '</p></a></li>'
    }).join('')
  }

  var move = function (step) {
    if (found.length === 0) return
    var items = results.querySelectorAll('li')
    items[chosen].removeAttribute('aria-selected')
    chosen = (chosen + step + items.length) % items.length
    items[chosen].setAttribute('aria-selected', 'true')
    items[chosen].scrollIntoView({ block: 'nearest' })
  }

  term.addEventListener('input', function () { show(look(term.value)) })

  if (openSearch) openSearch.addEventListener('click', open)

  document.addEventListener('keydown', function (event) {
    var typing = /^(INPUT|TEXTAREA|SELECT)$/.test(document.activeElement.tagName)
    if (search.hidden && !typing && (event.key === '/' || ((event.metaKey || event.ctrlKey) && event.key === 'k'))) {
      event.preventDefault()
      open()
      return
    }
    if (search.hidden) return
    if (event.key === 'Escape') { event.preventDefault(); close() }
    if (event.key === 'ArrowDown') { event.preventDefault(); move(1) }
    if (event.key === 'ArrowUp') { event.preventDefault(); move(-1) }
    if (event.key === 'Enter') {
      var choice = results.querySelector('li[aria-selected="true"] a')
      if (choice) { event.preventDefault(); window.location.href = choice.getAttribute('href') }
    }
    if (event.key === 'Tab') {
      event.preventDefault()
      term.focus()
    }
  })

  search.addEventListener('click', function (event) { if (event.target === search) close() })
}

/* ---------------------------------------------------------- indice lateral */

var contents = document.querySelector('.contents')
if (contents && 'IntersectionObserver' in window) {
  var links = {}
  contents.querySelectorAll('a').forEach(function (link) {
    links[link.getAttribute('href').slice(1)] = link
  })
  var observer = new IntersectionObserver(function (entries) {
    entries.forEach(function (entry) {
      var link = links[entry.target.id]
      if (link && entry.isIntersecting) {
        contents.querySelectorAll('a').forEach(function (other) { other.removeAttribute('aria-current') })
        link.setAttribute('aria-current', 'true')
      }
    })
  }, { rootMargin: '0px 0px -75% 0px' })
  document.querySelectorAll('article h2[id], article h3[id]').forEach(function (heading) {
    observer.observe(heading)
  })
}
