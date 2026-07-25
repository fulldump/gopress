(function () {
  'use strict';

  var list = document.getElementById('bookmarks-list');
  var status = document.getElementById('bookmarks-status');
  if (!list || !status) return;

  function element(tag, className, text) {
    var node = document.createElement(tag);
    if (className) node.className = className;
    if (text !== undefined) node.textContent = text;
    return node;
  }

  function formatDate(value) {
    var date = new Date(value);
    if (Number.isNaN(date.getTime())) return '';
    return new Intl.DateTimeFormat('es', { dateStyle: 'medium' }).format(date);
  }

  function articlePath(article) {
    return '/blog/' + encodeURIComponent(article.blogSlug) + '/article/' + encodeURIComponent(article.url);
  }

  function renderArticle(article) {
    var card = element('article', 'article-card bookmark-card');
    var heading = element('h3');
    var link = element('a', '', article.title);
    link.href = articlePath(article);
    heading.appendChild(link);
    card.appendChild(heading);

    if (article.contentSummary) {
      var excerpt = element('div', 'card-excerpt', article.contentSummary);
      card.appendChild(excerpt);
    }

    var meta = element('div', 'article-meta');
    var author = element('a', '', '@' + article.authorNick);
    author.href = '/blog/' + encodeURIComponent(article.blogSlug);
    meta.appendChild(author);
    meta.appendChild(element('span', '', formatDate(article.publishOn)));
    card.appendChild(meta);

    var remove = element('button', 'bookmark-remove', 'Quitar de guardados');
    remove.type = 'button';
    remove.addEventListener('click', async function () {
      remove.disabled = true;
      try {
        var response = await fetch('/v1/articles/' + encodeURIComponent(article.id) + '/bookmark', {
          method: 'DELETE',
          headers: window.gopressAuthHeaders({ Accept: 'application/json' })
        });
        if (response.status === 401) {
          window.gopressRequireLogin('/bookmarks');
          return;
        }
        if (!response.ok) throw new Error('No se pudo retirar el artículo.');
        var nextCard = card.nextElementSibling || card.previousElementSibling;
        card.remove();
        status.hidden = false;
        status.textContent = 'Artículo retirado de tus guardados.';
        if (!list.children.length) {
          showEmpty();
        } else if (nextCard) {
          nextCard.querySelector('a').focus();
        }
      } catch (error) {
        status.hidden = false;
        status.textContent = error.message;
      } finally {
        remove.disabled = false;
      }
    });
    card.appendChild(remove);
    return card;
  }

  function showEmpty() {
    status.hidden = false;
    status.className = 'empty-state bookmarks-status';
    status.innerHTML = '<h2>Aún no has guardado artículos.</h2><p>Usa el botón Guardar al pie de cualquier publicación para añadirla aquí.</p>';
  }

  async function loadBookmarks() {
    var me = window.gopressCurrentUser ? await window.gopressCurrentUser : null;
    if (!me || !me.id) {
      window.gopressRequireLogin('/bookmarks');
      return;
    }

    try {
      var response = await fetch('/v1/bookmarks', {
        headers: window.gopressAuthHeaders({ Accept: 'application/json' })
      });
      if (response.status === 401) {
        window.gopressRequireLogin('/bookmarks');
        return;
      }
      if (!response.ok) throw new Error('No se pudieron cargar los artículos guardados.');
      var bookmarks = await response.json();
      list.replaceChildren();
      if (!bookmarks.length) {
        showEmpty();
        return;
      }
      status.hidden = true;
      bookmarks.forEach(function (article) {
        list.appendChild(renderArticle(article));
      });
    } catch (error) {
      status.textContent = error.message;
    }
  }

  loadBookmarks();
})();
