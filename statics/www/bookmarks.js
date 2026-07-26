(function () {
  'use strict';

  var i18n = window.gopressI18n || { locale: 'es', t: function (key) { return key; } };
  var t = i18n.t;

  function element(tag, className, text) {
    var node = document.createElement(tag);
    if (className) node.className = className;
    if (text !== undefined) node.textContent = text;
    return node;
  }

  function formatDate(value) {
    var date = new Date(value);
    if (Number.isNaN(date.getTime())) return '';
    return new Intl.DateTimeFormat(i18n.locale, { dateStyle: 'medium' }).format(date);
  }

  function contextMeta(item) {
    var meta = element('div', 'article-meta');
    if (item.blog) {
      var blog = element('a', '', item.blog.name);
      blog.href = item.blog.publicUrl;
      meta.appendChild(blog);
    }
    if (item.article) {
      var article = element('a', '', item.article.title);
      article.href = item.article.publicUrl;
      meta.appendChild(article);
    }
    meta.appendChild(element('span', '', formatDate(item.createdOn || item.publishOn)));
    return meta;
  }

  function renderItem(kind, item, removeItem) {
    var card = element('article', 'article-card bookmark-card saved-card');
    var heading = element('h3');
    var link;
    if (kind === 'articles') {
      link = element('a', '', item.title);
      link.href = item.publicUrl;
      heading.appendChild(link);
      card.appendChild(heading);
      if (item.contentSummary) card.appendChild(element('div', 'card-excerpt', item.contentSummary));
      var articleMeta = element('div', 'article-meta');
      articleMeta.appendChild(element('span', '', item.authorNick));
      articleMeta.appendChild(element('span', '', formatDate(item.publishOn)));
      card.appendChild(articleMeta);
    } else if (kind === 'blogs') {
      link = element('a', '', item.name);
      link.href = item.publicUrl;
      heading.appendChild(link);
      card.appendChild(heading);
      if (item.description) card.appendChild(element('div', 'card-excerpt', item.description));
    } else {
      link = element('a', '', item.name + ': ' + String(item.text || '').slice(0, 100));
      link.href = item.url;
      heading.appendChild(link);
      card.appendChild(heading);
      card.appendChild(element('div', 'card-excerpt', item.text));
      card.appendChild(contextMeta(item));
    }

    if (kind !== 'own-comments') {
      var remove = element('button', 'bookmark-remove', t('saved.remove'));
      remove.type = 'button';
      remove.addEventListener('click', function () { removeItem(item, card, remove); });
      card.appendChild(remove);
    }
    return card;
  }

  function initSection(section) {
    var kind = section.dataset.kind;
    var list = section.querySelector('[data-saved-list]');
    var status = section.querySelector('[data-saved-status]');
    var more = section.querySelector('[data-saved-more]');
    var offset = 0;

    function showEmpty() {
      status.hidden = false;
      status.className = 'empty-state saved-status';
      status.textContent = t('saved.empty_' + kind.replace('-', '_'));
    }

    async function removeItem(item, card, button) {
      button.disabled = true;
      var targetKind = kind === 'articles' ? 'articles' : (kind === 'blogs' ? 'blogs' : 'comments');
      var focusCard = card.nextElementSibling || card.previousElementSibling;
      try {
        var response = await fetch('/v1/' + targetKind + '/' + encodeURIComponent(item.id) + '/bookmark', {
          method: 'DELETE', headers: window.gopressAuthHeaders({ Accept: 'application/json' })
        });
        if (response.status === 401) return window.gopressRequireLogin('/bookmarks');
        if (!response.ok) throw new Error(t('saved.remove_failed'));
        card.remove();
        offset = Math.max(0, offset - 1);
        status.hidden = false;
        status.className = 'saved-status';
        status.textContent = t('saved.removed');
        if (!list.children.length && more.hidden) showEmpty();
        var focusTarget = focusCard && focusCard.querySelector('a, button');
        if (!focusTarget && !more.hidden) focusTarget = more;
        if (!focusTarget) {
          focusTarget = section.querySelector('h2');
          if (focusTarget) focusTarget.tabIndex = -1;
        }
        if (focusTarget) focusTarget.focus();
      } catch (error) {
        status.hidden = false;
        status.textContent = error.message;
      } finally {
        button.disabled = false;
      }
    }

    async function load(append) {
      status.hidden = false;
      status.className = 'saved-status';
      status.textContent = t('common.loading');
      more.disabled = true;
      try {
        var response = await fetch('/v1/saved/' + kind + '?offset=' + offset, {
          headers: window.gopressAuthHeaders({ Accept: 'application/json' })
        });
        if (response.status === 401) return window.gopressRequireLogin('/bookmarks');
        if (!response.ok) throw new Error(t('saved.load_failed'));
        var payload = await response.json();
        if (!append) list.replaceChildren();
        (payload.items || []).forEach(function (item) {
          list.appendChild(renderItem(kind, item, removeItem));
        });
        offset = Number(payload.nextOffset) || 0;
        more.hidden = !payload.hasMore;
        more.disabled = false;
        more.textContent = t('pagination.load_more');
        if (!list.children.length) showEmpty();
        else status.hidden = true;
      } catch (error) {
        status.hidden = false;
        status.textContent = error.message;
        more.hidden = false;
        more.disabled = false;
        more.textContent = t('saved.retry');
      }
    }

    more.addEventListener('click', function () { load(list.children.length > 0); });
    load(false);
  }

  (window.gopressCurrentUser || Promise.resolve(null)).then(function (me) {
    if (!me || !me.id) {
      window.gopressRequireLogin('/bookmarks');
      return;
    }
    document.querySelectorAll('[data-saved-section]').forEach(initSection);
  });
})();
