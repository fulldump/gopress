(function () {
  'use strict';

  var login = document.getElementById('login');
  var logout = document.getElementById('logout');
  var currentUser = null;
  var localAuthHosts = ['localhost', '127.0.0.1', '::1'];

  window.gopressAuthHeaders = function (initial) {
    var headers = new Headers(initial || {});
    if (localAuthHosts.includes(window.location.hostname) && currentUser && currentUser.id) {
      headers.set('X-Glue-Authentication', JSON.stringify({
        session: { id: 'local-development' },
        user: {
          id: currentUser.id,
          nick: currentUser.nick || '',
          picture: currentUser.picture || '',
          email: currentUser.email || ''
        }
      }));
    }
    return headers;
  };

  function showAnonymous() {
    if (login) login.hidden = false;
    if (logout) logout.hidden = true;
  }

  function loginUrl(target) {
    return '/auth/login?redirect=' + encodeURIComponent(target);
  }

  window.gopressRequireLogin = function (target) {
    window.location.href = loginUrl(target);
  };

  if (login) {
    var loginLink = login.querySelector('a[href="/auth/login"]');
    if (loginLink) {
      loginLink.href = loginUrl(window.location.pathname + window.location.search + window.location.hash);
    }
  }

  window.gopressCurrentUser = fetch('/auth/me', { headers: { Accept: 'application/json' } })
    .then(function (response) {
      if (!response.ok) throw new Error('No session');
      return response.json();
    })
    .then(function (me) {
      if (me.error || !me.nick) {
        showAnonymous();
        return null;
      }

      currentUser = me;
      if (login) login.hidden = true;
      if (logout) logout.hidden = false;
      var nick = document.getElementById('user_nick');
      if (nick) nick.textContent = me.nick;

      var picture = document.getElementById('user_picture');
      if (picture && me.picture) {
        picture.src = me.picture;
        picture.alt = 'Foto de ' + me.nick;
        picture.hidden = false;
      }
      return me;
    })
    .catch(function () {
      showAnonymous();
      return null;
    });

  function normalizeSearch(value) {
    return String(value || '').toLowerCase().normalize('NFD').replace(/[\u0300-\u036f]/g, '').trim();
  }

  function initBlogSearch() {
    var input = document.getElementById('blog-search');
    if (!input) return;
    var cards = Array.prototype.slice.call(document.querySelectorAll('.blog-directory-card'));
    var status = document.getElementById('blog-search-status');
    var empty = document.getElementById('blog-search-empty');
    input.addEventListener('input', function () {
      var query = normalizeSearch(input.value);
      var visible = 0;
      cards.forEach(function (card) {
        var matches = !query || normalizeSearch(card.textContent).includes(query);
        card.hidden = !matches;
        if (matches) visible += 1;
      });
      if (empty) empty.hidden = visible !== 0;
      if (status) status.textContent = query ? visible + (visible === 1 ? ' resultado' : ' resultados') : '';
    });
  }

  function initArticleSearch() {
    var actions = document.querySelector('.auth-actions');
    if (!actions || !window.HTMLDialogElement) return;
    var trigger = document.createElement('button');
    trigger.type = 'button';
    trigger.className = 'global-search-button';
    trigger.setAttribute('aria-label', 'Buscar artículos');
    trigger.innerHTML = '<svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="11" cy="11" r="6.5"></circle><path d="m16 16 4 4"></path></svg>';
    actions.insertBefore(trigger, actions.firstChild);

    var dialog = document.createElement('dialog');
    dialog.className = 'share-dialog search-dialog';
    dialog.setAttribute('aria-labelledby', 'article-search-title');
    dialog.innerHTML = '<div class="share-dialog-header"><div><span class="eyebrow">Explorar GoPress</span><h2 id="article-search-title">Buscar artículos</h2></div><button class="share-dialog-close" type="button" aria-label="Cerrar"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="m6 6 12 12M18 6 6 18"></path></svg></button></div><label class="sr-only" for="article-search-input">Título, autor, blog o tag</label><input class="article-search-input" id="article-search-input" type="search" placeholder="Título, autor, blog o tag" autocomplete="off"><p class="article-search-status" role="status" aria-live="polite">Escribe al menos dos caracteres.</p><div class="article-search-results"></div>';
    document.body.appendChild(dialog);

    var input = dialog.querySelector('.article-search-input');
    var close = dialog.querySelector('.share-dialog-close');
    var status = dialog.querySelector('.article-search-status');
    var results = dialog.querySelector('.article-search-results');
    var timer = null;
    var activeRequest = null;

    function setStatus(message) {
      status.textContent = message;
    }

    function renderResults(items) {
      results.textContent = '';
      items.forEach(function (article) {
        var link = document.createElement('a');
        link.className = 'article-search-result';
        link.href = article.publicUrl;
        var title = document.createElement('strong');
        title.textContent = article.title || 'Sin título';
        var meta = document.createElement('span');
        meta.textContent = (article.blogName || article.blogSlug) + (article.authorNick ? ' · ' + article.authorNick : '');
        link.appendChild(title);
        link.appendChild(meta);
        results.appendChild(link);
      });
    }

    function search() {
      var query = input.value.trim();
      if (query.length < 2) {
        if (activeRequest) activeRequest.abort();
        renderResults([]);
        setStatus('Escribe al menos dos caracteres.');
        return;
      }
      if (activeRequest) activeRequest.abort();
      var requestController = new AbortController();
      activeRequest = requestController;
      setStatus('Buscando...');
      fetch('/v1/search/articles?q=' + encodeURIComponent(query), {
        headers: { Accept: 'application/json' }, signal: requestController.signal
      }).then(function (response) {
        if (!response.ok) throw new Error('Search failed');
        return response.json();
      }).then(function (payload) {
        if (activeRequest !== requestController) return;
        var items = payload && Array.isArray(payload.articles) ? payload.articles : [];
        renderResults(items);
        setStatus(items.length ? items.length + (items.length === 1 ? ' artículo' : ' artículos') : 'No se encontraron artículos.');
      }).catch(function (error) {
        if (activeRequest === requestController && error.name !== 'AbortError') {
          renderResults([]);
          setStatus('No se pudo completar la búsqueda.');
        }
      });
    }

    trigger.addEventListener('click', function () {
      dialog.showModal();
      window.setTimeout(function () { input.focus(); }, 0);
    });
    close.addEventListener('click', function () { dialog.close(); });
    dialog.addEventListener('close', function () {
      window.clearTimeout(timer);
      if (activeRequest) activeRequest.abort();
      activeRequest = null;
    });
    dialog.addEventListener('click', function (event) {
      if (event.target === dialog) dialog.close();
    });
    input.addEventListener('input', function () {
      window.clearTimeout(timer);
      if (activeRequest) activeRequest.abort();
      activeRequest = null;
      renderResults([]);
      timer = window.setTimeout(search, 250);
    });
  }

  initBlogSearch();
  initArticleSearch();
})();
