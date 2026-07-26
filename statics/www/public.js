(function () {
  'use strict';

  var login = document.getElementById('login');
  var logout = document.getElementById('logout');
  var currentUser = null;
  var localAuthHosts = ['localhost', '127.0.0.1', '::1'];
  var messages = {};
  var messagesNode = document.getElementById('gopress-i18n');
  if (messagesNode) {
    try { messages = JSON.parse(messagesNode.textContent); } catch (_) {}
  }
  function tr(key) { return messages[key] || key; }
  window.gopressI18n = { locale: document.documentElement.lang || 'es', t: tr };

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

  async function copyShareUrl(url) {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(url);
      return;
    }
    var input = document.createElement('textarea');
    input.value = url;
    input.setAttribute('readonly', '');
    input.style.position = 'fixed';
    input.style.opacity = '0';
    document.body.appendChild(input);
    input.select();
    var copied = document.execCommand('copy');
    input.remove();
    if (!copied) throw new Error('Copy failed');
  }

  window.gopressShare = async function (url, title) {
    if (navigator.share) {
      try {
        await navigator.share({ title: title || document.title, url: url });
        return 'shared';
      } catch (error) {
        if (error && error.name === 'AbortError') return 'cancelled';
      }
    }
    await copyShareUrl(url);
    return 'copied';
  };
  window.gopressCopyShareUrl = copyShareUrl;

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
        picture.alt = me.nick;
        picture.hidden = false;
      }
      return me;
    })
    .catch(function () {
      showAnonymous();
      return null;
    });

  document.querySelectorAll('.locale-switcher a').forEach(function (link) {
    link.addEventListener('click', async function (event) {
      var destination = link.href;
      var language = new URL(destination, window.location.href).searchParams.get('lang') || link.hreflang || link.textContent.trim().toLowerCase();
      event.preventDefault();
      var user = window.gopressCurrentUser ? await window.gopressCurrentUser : null;
      if (!user || !user.id || !['es', 'en', 'zh'].includes(language)) {
        window.location.assign(destination);
        return;
      }
      try {
        await fetch('/v1/preferences', {
          method: 'PUT', credentials: 'same-origin', headers: window.gopressAuthHeaders({ Accept: 'application/json', 'Content-Type': 'application/json' }),
          body: JSON.stringify({ locale: language })
        });
      } catch (_) {}
      window.location.assign(destination);
    });
  });

  function normalizeSearch(value) {
    return String(value || '').toLowerCase().normalize('NFD').replace(/[\u0300-\u036f]/g, '').trim();
  }

  var reapplyBlogSearch = function () {};

  function initBlogSearch() {
    var input = document.getElementById('blog-search');
    if (!input) return;
    var status = document.getElementById('blog-search-status');
    var empty = document.getElementById('blog-search-empty');
    reapplyBlogSearch = function () {
      var cards = Array.prototype.slice.call(document.querySelectorAll('.blog-directory-card'));
      var query = normalizeSearch(input.value);
      var visible = 0;
      cards.forEach(function (card) {
        var matches = !query || normalizeSearch(card.textContent).includes(query);
        card.hidden = !matches;
        if (matches) visible += 1;
      });
      var nextPage = document.querySelector('[data-pagination-control] [data-load-more]');
      if (empty) empty.hidden = visible !== 0 || Boolean(nextPage);
      if (query && visible === 0 && nextPage) {
        if (status) status.textContent = tr('pagination.loading');
        nextPage.click();
      } else if (status) {
        status.textContent = query ? visible + ' ' + tr(visible === 1 ? 'search.result' : 'search.results') : '';
      }
    };
    input.addEventListener('input', reapplyBlogSearch);
  }

  function initPagination() {
    document.addEventListener('click', function (event) {
      var link = event.target.closest && event.target.closest('[data-load-more]');
      if (!link) return;
      event.preventDefault();
      var control = link.closest('[data-pagination-control]');
      var list = document.querySelector('[data-pagination-list]');
      if (!control || !list || control.dataset.loading === 'true') return;
      control.dataset.loading = 'true';
      link.setAttribute('aria-disabled', 'true');
      var preserveSearchFocus = document.activeElement === document.getElementById('blog-search');
      var status = control.querySelector('[data-pagination-status]');
      var originalText = link.textContent;
      link.textContent = tr('pagination.loading');
      if (status) status.textContent = '';

      fetch(link.href, { headers: { Accept: 'text/html' }, credentials: 'same-origin' })
        .then(function (response) {
          if (!response.ok) throw new Error('Pagination failed');
          return response.text();
        })
        .then(function (html) {
          var page = new DOMParser().parseFromString(html, 'text/html');
          var sourceList = page.querySelector('[data-pagination-list]');
          if (!sourceList) throw new Error('Pagination list missing');
          var known = new Set(Array.prototype.map.call(list.querySelectorAll('[data-pagination-item]'), function (item) {
            return item.getAttribute('data-item-id');
          }));
          var firstAdded = null;
          sourceList.querySelectorAll('[data-pagination-item]').forEach(function (item) {
            var id = item.getAttribute('data-item-id');
            if (!id || !known.has(id)) {
              var added = document.importNode(item, true);
              list.appendChild(added);
              if (!firstAdded) firstAdded = added;
              if (id) known.add(id);
            }
          });
          var nextControl = page.querySelector('[data-pagination-control]');
          if (nextControl) control.replaceWith(document.importNode(nextControl, true));
          else control.remove();
          reapplyBlogSearch();
          if (firstAdded && !preserveSearchFocus) {
            var focusTarget = firstAdded.querySelector('a, button, [tabindex]');
            if (focusTarget) focusTarget.focus();
          }
        })
        .catch(function () {
          control.dataset.loading = 'false';
          link.removeAttribute('aria-disabled');
          link.textContent = originalText;
          if (status) status.textContent = tr('pagination.failed');
        });
    });
  }

  function initArticleSearch() {
    var actions = document.querySelector('.auth-actions');
    if (!actions || !window.HTMLDialogElement) return;
    var trigger = document.createElement('button');
    trigger.type = 'button';
    trigger.className = 'global-search-button';
    trigger.setAttribute('aria-label', tr('search.articles'));
    trigger.innerHTML = '<svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="11" cy="11" r="6.5"></circle><path d="m16 16 4 4"></path></svg>';
    actions.insertBefore(trigger, actions.firstChild);

    var dialog = document.createElement('dialog');
    dialog.className = 'share-dialog search-dialog';
    dialog.setAttribute('aria-labelledby', 'article-search-title');
    dialog.innerHTML = '<div class="share-dialog-header"><div><span class="eyebrow">GoPress</span><h2 id="article-search-title"></h2></div><button class="share-dialog-close" type="button"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="m6 6 12 12M18 6 6 18"></path></svg></button></div><label class="sr-only" for="article-search-input"></label><input class="article-search-input" id="article-search-input" type="search" autocomplete="off"><p class="article-search-status" role="status" aria-live="polite"></p><div class="article-search-results"></div><button class="load-more-button article-search-more" type="button" hidden></button>';
    document.body.appendChild(dialog);

    var input = dialog.querySelector('.article-search-input');
    var close = dialog.querySelector('.share-dialog-close');
    var status = dialog.querySelector('.article-search-status');
    var results = dialog.querySelector('.article-search-results');
    var more = dialog.querySelector('.article-search-more');
    dialog.querySelector('#article-search-title').textContent = tr('search.articles');
    dialog.querySelector('.share-dialog-close').setAttribute('aria-label', tr('common.close'));
    dialog.querySelector('label').textContent = tr('search.query');
    input.placeholder = tr('search.query');
    status.textContent = tr('search.minimum');
    more.textContent = tr('pagination.load_more');
    var timer = null;
    var activeRequest = null;
    var nextOffset = 0;

    function setStatus(message) {
      status.textContent = message;
    }

    function renderResults(items, append) {
      if (!append) results.textContent = '';
      items.forEach(function (article) {
        var link = document.createElement('a');
        link.className = 'article-search-result';
        link.href = article.publicUrl;
        var title = document.createElement('strong');
        title.textContent = article.title || tr('common.untitled');
        var meta = document.createElement('span');
        meta.textContent = (article.blogName || article.blogSlug) + (article.authorNick ? ' · ' + article.authorNick : '');
        link.appendChild(title);
        link.appendChild(meta);
        results.appendChild(link);
      });
    }

    function search(offset, append) {
      var query = input.value.trim();
      if (query.length < 2) {
        if (activeRequest) activeRequest.abort();
        renderResults([], false);
        more.hidden = true;
        setStatus(tr('search.minimum'));
        return;
      }
      if (activeRequest) activeRequest.abort();
      var requestController = new AbortController();
      activeRequest = requestController;
      setStatus(offset ? tr('pagination.loading') : tr('search.searching'));
      more.disabled = true;
      fetch('/v1/search/articles?q=' + encodeURIComponent(query) + '&offset=' + offset, {
        headers: { Accept: 'application/json' }, signal: requestController.signal
      }).then(function (response) {
        if (!response.ok) throw new Error('Search failed');
        return response.json();
      }).then(function (payload) {
        if (activeRequest !== requestController) return;
        var items = payload && Array.isArray(payload.items) ? payload.items : (payload && Array.isArray(payload.articles) ? payload.articles : []);
        renderResults(items, append);
        nextOffset = Number(payload && payload.nextOffset) || 0;
        more.hidden = !(payload && payload.hasMore);
        more.disabled = false;
        var count = results.children.length;
        setStatus(count ? count + ' ' + tr(count === 1 ? 'search.article' : 'search.articles_count') : tr('search.none'));
        activeRequest = null;
      }).catch(function (error) {
        if (activeRequest === requestController && error.name !== 'AbortError') {
          if (!append) renderResults([], false);
          more.disabled = false;
          more.hidden = false;
          more.textContent = tr('pagination.failed');
          setStatus(append ? tr('pagination.failed') : tr('search.failed'));
        }
        if (activeRequest === requestController) activeRequest = null;
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
      more.textContent = tr('pagination.load_more');
    });
    dialog.addEventListener('click', function (event) {
      if (event.target === dialog) dialog.close();
    });
    input.addEventListener('input', function () {
      window.clearTimeout(timer);
      if (activeRequest) activeRequest.abort();
      activeRequest = null;
      renderResults([]);
      more.hidden = true;
      nextOffset = 0;
      timer = window.setTimeout(function () { search(0, false); }, 250);
    });
    more.addEventListener('click', function () {
      if (activeRequest) return;
      more.textContent = tr('pagination.load_more');
      search(nextOffset, true);
    });
  }

  initBlogSearch();
  initPagination();
  initArticleSearch();
})();
