(function () {
  'use strict';

  var section = document.getElementById('comments');
  if (!section) return;

  var endpoint = section.dataset.endpoint;
  var list = document.getElementById('comments-list');
  var status = document.getElementById('comments-status');
  var count = document.getElementById('comments-count');
  var rootForm = document.getElementById('comment-form');
  var openReply = null;

  function element(tag, className, text) {
    var node = document.createElement(tag);
    if (className) node.className = className;
    if (text !== undefined) node.textContent = text;
    return node;
  }

  function appendInline(parent, text) {
    var pattern = /(`[^`\n]+`|\*\*[^*\n]+\*\*|\*[^*\n]+\*|\[[^\]\n]+\]\([^\s)]+\))/g;
    var position = 0;
    var match;

    while ((match = pattern.exec(text)) !== null) {
      parent.appendChild(document.createTextNode(text.slice(position, match.index)));
      var token = match[0];

      if (token.charAt(0) === '`') {
        parent.appendChild(element('code', '', token.slice(1, -1)));
      } else if (token.slice(0, 2) === '**') {
        var strong = element('strong');
        appendInline(strong, token.slice(2, -2));
        parent.appendChild(strong);
      } else if (token.charAt(0) === '*') {
        var emphasis = element('em');
        appendInline(emphasis, token.slice(1, -1));
        parent.appendChild(emphasis);
      } else {
        var linkMatch = token.match(/^\[([^\]]+)\]\(([^)]+)\)$/);
        var link = element('a', '', linkMatch[1]);
        try {
          var url = new URL(linkMatch[2], window.location.href);
          if (url.protocol === 'http:' || url.protocol === 'https:' || url.protocol === 'mailto:') {
            link.href = url.href;
            if (url.origin !== window.location.origin) {
              link.target = '_blank';
              link.rel = 'nofollow noopener noreferrer';
            }
          }
        } catch (error) {
          // Invalid URLs remain plain link text without a destination.
        }
        parent.appendChild(link);
      }

      position = pattern.lastIndex;
    }

    parent.appendChild(document.createTextNode(text.slice(position)));
  }

  function isBlockStart(line) {
    return /^\s*(```|~~~)/.test(line) ||
      /^\s{0,3}#{1,6}\s+/.test(line) ||
      /^\s*>\s?/.test(line) ||
      /^\s*[-+*]\s+/.test(line) ||
      /^\s*\d+\.\s+/.test(line) ||
      /^\s*(---+|___+)\s*$/.test(line);
  }

  function renderMarkdown(markdown) {
    var container = element('div', 'comment-body');
    var lines = String(markdown || '').replace(/\r\n?/g, '\n').split('\n');
    var index = 0;

    while (index < lines.length) {
      var line = lines[index];
      if (!line.trim()) {
        index += 1;
        continue;
      }

      var fence = line.match(/^\s*(`{3,}|~{3,})([\w.+-]*)\s*$/);
      if (fence) {
        var marker = fence[1];
        var codeLines = [];
        index += 1;
        while (index < lines.length && !lines[index].trim().startsWith(marker)) {
          codeLines.push(lines[index]);
          index += 1;
        }
        if (index < lines.length) index += 1;
        var pre = element('pre');
        var code = element('code', '', codeLines.join('\n'));
        if (fence[2]) code.className = 'language-' + fence[2];
        pre.appendChild(code);
        container.appendChild(pre);
        continue;
      }

      var heading = line.match(/^\s{0,3}(#{1,6})\s+(.+)$/);
      if (heading) {
        var headingNode = element('h' + Math.min(6, heading[1].length + 2));
        appendInline(headingNode, heading[2]);
        container.appendChild(headingNode);
        index += 1;
        continue;
      }

      if (/^\s*(---+|___+)\s*$/.test(line)) {
        container.appendChild(element('hr'));
        index += 1;
        continue;
      }

      if (/^\s*>\s?/.test(line)) {
        var quoteLines = [];
        while (index < lines.length && /^\s*>\s?/.test(lines[index])) {
          quoteLines.push(lines[index].replace(/^\s*>\s?/, ''));
          index += 1;
        }
        var quote = element('blockquote');
        appendInline(quote, quoteLines.join('\n'));
        container.appendChild(quote);
        continue;
      }

      var unordered = /^\s*[-+*]\s+/.test(line);
      var ordered = /^\s*\d+\.\s+/.test(line);
      if (unordered || ordered) {
        var items = element(ordered ? 'ol' : 'ul');
        var itemPattern = ordered ? /^\s*\d+\.\s+(.+)$/ : /^\s*[-+*]\s+(.+)$/;
        while (index < lines.length) {
          var itemMatch = lines[index].match(itemPattern);
          if (!itemMatch) break;
          var item = element('li');
          appendInline(item, itemMatch[1]);
          items.appendChild(item);
          index += 1;
        }
        container.appendChild(items);
        continue;
      }

      var paragraphLines = [line];
      index += 1;
      while (index < lines.length && lines[index].trim() && !isBlockStart(lines[index])) {
        paragraphLines.push(lines[index]);
        index += 1;
      }
      var paragraph = element('p');
      appendInline(paragraph, paragraphLines.join('\n'));
      container.appendChild(paragraph);
    }

    return container;
  }

  function formatDate(value) {
    var date = new Date(value);
    if (Number.isNaN(date.getTime())) return '';
    return new Intl.DateTimeFormat('es', {
      dateStyle: 'medium',
      timeStyle: 'short'
    }).format(date);
  }

  function makeForm(parentId) {
    var form = element('form', 'comment-form comment-reply-form');
    form.innerHTML = '<div class="comment-fields">' +
      '<label>Nombre<input type="text" name="name" maxlength="100" autocomplete="name" required></label>' +
      '<label>Email <span>(opcional, no será público)</span><input type="email" name="email" maxlength="254" autocomplete="email"></label>' +
      '</div>' +
      '<label>Respuesta<textarea name="text" rows="4" maxlength="10000" required></textarea></label>' +
      '<div class="comment-form-footer"><button type="button" class="comment-cancel">Cancelar</button><button type="submit">Responder</button></div>' +
      '<p class="comment-form-message" role="alert"></p>';

    form.elements.name.value = rootForm.elements.name.value;
    form.elements.email.value = rootForm.elements.email.value;
    form.querySelector('.comment-cancel').addEventListener('click', function () {
      form.remove();
      openReply = null;
    });
    bindForm(form, parentId);
    return form;
  }

  function renderComment(comment, canReply) {
    var item = element('article', 'comment');
    var header = element('header', 'comment-header');
    var avatar = element('span', 'comment-avatar', (comment.name || '?').trim().charAt(0).toUpperCase());
    var identity = element('div', 'comment-identity');
    identity.appendChild(element('strong', '', comment.name));
    identity.appendChild(element('time', '', formatDate(comment.createdOn)));
    header.appendChild(avatar);
    header.appendChild(identity);
    item.appendChild(header);
    item.appendChild(renderMarkdown(comment.text));

    if (canReply) {
      var replyButton = element('button', 'comment-reply', 'Responder');
      replyButton.type = 'button';
      replyButton.addEventListener('click', function () {
        if (openReply) openReply.remove();
        var form = makeForm(comment.id);
        item.appendChild(form);
        openReply = form;
        form.elements.text.focus();
      });
      item.appendChild(replyButton);
    }

    if (comment.replies && comment.replies.length) {
      var replies = element('div', 'comment-replies');
      comment.replies.forEach(function (reply) {
        replies.appendChild(renderComment(reply, false));
      });
      item.appendChild(replies);
    }
    return item;
  }

  function commentCount(comments) {
    return comments.reduce(function (total, comment) {
      return total + 1 + (comment.replies ? comment.replies.length : 0);
    }, 0);
  }

  function renderComments(comments) {
    list.replaceChildren();
    var total = commentCount(comments);
    count.textContent = total ? '(' + total + ')' : '';
    if (!comments.length) {
      status.textContent = 'Todavía no hay comentarios. Abre la conversación.';
      status.hidden = false;
      return;
    }

    status.hidden = true;
    comments.forEach(function (comment) {
      list.appendChild(renderComment(comment, true));
    });
  }

  async function loadComments() {
    status.hidden = false;
    status.textContent = 'Cargando comentarios...';
    try {
      var response = await fetch(endpoint, { headers: { Accept: 'application/json' } });
      if (!response.ok) throw new Error('No se pudieron cargar los comentarios.');
      renderComments(await response.json());
    } catch (error) {
      list.replaceChildren();
      status.textContent = error.message;
    }
  }

  function bindForm(form, parentId) {
    form.addEventListener('submit', async function (event) {
      event.preventDefault();
      var message = form.querySelector('.comment-form-message');
      var submit = form.querySelector('button[type="submit"]');
      var data = new FormData(form);
      message.textContent = '';
      submit.disabled = true;

      try {
        var response = await fetch(endpoint, {
          method: 'POST',
          headers: {
            Accept: 'application/json',
            'Content-Type': 'application/json'
          },
          body: JSON.stringify({
            parentId: parentId || '',
            name: String(data.get('name') || '').trim(),
            email: String(data.get('email') || '').trim(),
            text: String(data.get('text') || '').trim()
          })
        });
        var payload = await response.json().catch(function () { return {}; });
        if (!response.ok) {
          throw new Error(payload.error && payload.error.description ? payload.error.description : 'No se pudo publicar el comentario.');
        }

        if (!parentId) {
          form.elements.text.value = '';
        } else {
          form.remove();
          openReply = null;
        }
        await loadComments();
      } catch (error) {
        message.textContent = error.message;
      } finally {
        submit.disabled = false;
      }
    });
  }

  bindForm(rootForm, '');
  loadComments();
})();
