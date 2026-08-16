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
  botao.addEventListener('click', function () {
    navigator.clipboard.writeText(bloco.innerText).then(function () {
      botao.textContent = 'copiado'
      setTimeout(function () { botao.textContent = 'copiar' }, 1500)
    }, function () {
      botao.textContent = 'copie à mão'
    })
  })
  caixa.appendChild(botao)
})

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
