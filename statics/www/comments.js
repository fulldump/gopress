(function () {
  'use strict';

  var section = document.getElementById('comments');
  var i18n = window.gopressI18n || { locale: 'es', t: function (key) { return key; } };
  var t = i18n.t;
  if (!section) return;

  var endpoint = section.dataset.endpoint;
  var list = document.getElementById('comments-list');
  var status = document.getElementById('comments-status');
  var count = document.getElementById('comments-count');
  var more = document.getElementById('comments-more');
  var rootForm = document.getElementById('comment-form');
  var openReply = null;
  var loadedComments = [];
  var nextOffset = 0;
  var hasMore = false;
  var commentsLoading = false;

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
    return new Intl.DateTimeFormat(i18n.locale, {
      dateStyle: 'medium',
      timeStyle: 'short'
    }).format(date);
  }

  function makeForm(parentId) {
    var form = element('form', 'comment-form comment-reply-form');
    form.innerHTML = '<div class="comment-fields">' +
      '<label>' + t('comments.name') + '<input type="text" name="name" maxlength="100" autocomplete="name" required></label>' +
      '<label>' + t('comments.email') + ' <small>' + t('comments.optional') + '</small><input type="email" name="email" maxlength="254" autocomplete="email"></label>' +
      '</div>' +
      '<label>' + t('comments.reply_label') + '<textarea name="text" rows="4" maxlength="10000" required></textarea></label>' +
      '<div class="comment-form-footer"><button type="button" class="comment-cancel">' + t('comments.cancel') + '</button><button type="submit">' + t('comments.reply') + '</button></div>' +
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
    var fragment = '';
    try { fragment = new URL(comment.url, window.location.href).hash.slice(1); } catch (_) {}
    item.id = fragment || ('comment-' + String(comment.id || '').replace(/^comment_/, ''));
    item.tabIndex = -1;
    var header = element('header', 'comment-header');
    var avatar = element('span', 'comment-avatar', (comment.name || '?').trim().charAt(0).toUpperCase());
    var identity = element('div', 'comment-identity');
    identity.appendChild(element('strong', '', comment.name));
    var time = element('time', '', formatDate(comment.createdOn));
    time.dateTime = comment.createdOn;
    identity.appendChild(time);
    header.appendChild(avatar);
    header.appendChild(identity);
    item.appendChild(header);
    item.appendChild(renderMarkdown(comment.text));

    var actions = element('div', 'comment-actions');
    if (canReply) {
      var replyButton = element('button', 'comment-reply', t('comments.reply'));
      replyButton.type = 'button';
      replyButton.addEventListener('click', function () {
        if (openReply) openReply.remove();
        var form = makeForm(comment.id);
        item.appendChild(form);
        openReply = form;
        form.elements.text.focus();
      });
      actions.appendChild(replyButton);
    }

    var feedback = comment.feedback || {};
    var applaud = element('button', 'comment-action');
    applaud.type = 'button';
    applaud.innerHTML = '<span aria-hidden="true">👏</span> <span class="comment-like-count"></span>';
    var save = element('button', 'comment-action', t('feedback.save'));
    save.type = 'button';
    var share = element('button', 'comment-action', t('feedback.share'));
    share.type = 'button';
    var actionStatus = element('span', 'sr-only');
    actionStatus.setAttribute('role', 'status');
    actionStatus.setAttribute('aria-live', 'polite');

    function updateButtons(value) {
      feedback = value || {};
      applaud.setAttribute('aria-pressed', feedback.liked ? 'true' : 'false');
      applaud.setAttribute('aria-label', feedback.liked ? t('feedback.remove_applause') : t('feedback.applaud'));
      applaud.querySelector('.comment-like-count').textContent = String(feedback.likes || 0);
      save.setAttribute('aria-pressed', feedback.bookmarked ? 'true' : 'false');
      save.textContent = feedback.bookmarked ? t('feedback.remove_saved') : t('feedback.save');
    }

    async function mutate(button, resource) {
      var me = window.gopressCurrentUser ? await window.gopressCurrentUser : null;
      if (!me || !me.id) {
        window.gopressRequireLogin(window.location.pathname + window.location.search + '#' + item.id);
        return;
      }
      button.disabled = true;
      actionStatus.textContent = '';
      try {
        var active = button.getAttribute('aria-pressed') === 'true';
        var response = await fetch('/v1/comments/' + encodeURIComponent(comment.id) + '/' + resource, {
          method: active ? 'DELETE' : 'PUT',
          headers: window.gopressAuthHeaders({ Accept: 'application/json' })
        });
        if (response.status === 401) {
          window.gopressRequireLogin(window.location.pathname + window.location.search + '#' + item.id);
          return;
        }
        var payload = await response.json().catch(function () { return {}; });
        if (!response.ok) throw new Error(payload.error && payload.error.description ? payload.error.description : t('feedback.update_failed'));
        comment.feedback = payload;
        updateButtons(payload);
        actionStatus.textContent = resource === 'like'
          ? (payload.liked ? t('feedback.applause_added') : t('feedback.applause_removed'))
          : (payload.bookmarked ? t('feedback.saved_added') : t('feedback.saved_removed'));
      } catch (error) {
        actionStatus.textContent = error.message;
      } finally {
        button.disabled = false;
      }
    }

    applaud.addEventListener('click', function () { mutate(applaud, 'like'); });
    save.addEventListener('click', function () { mutate(save, 'bookmark'); });
    share.addEventListener('click', async function () {
      try {
        var result = await window.gopressShare(new URL(comment.url, window.location.href).href, t('comments.share_title'));
        if (result === 'copied') actionStatus.textContent = t('feedback.link_copied');
      } catch (_) {
        actionStatus.textContent = t('feedback.copy_failed');
      }
    });
    updateButtons(feedback);
    actions.appendChild(applaud);
    actions.appendChild(save);
    actions.appendChild(share);
    actions.appendChild(actionStatus);
    item.appendChild(actions);

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
    appendComments(comments);
  }

  function appendComments(comments) {
    comments.forEach(function (comment) {
      list.appendChild(renderComment(comment, true));
    });
    var total = commentCount(loadedComments);
    count.textContent = total ? '(' + total + ')' : '';
    if (!loadedComments.length) {
      status.textContent = t('comments.empty');
      status.hidden = false;
      return;
    }

    status.hidden = true;
  }

  function focusHashTarget() {
    if (!window.location.hash || !window.location.hash.startsWith('#comment-')) return false;
    var target = document.getElementById(window.location.hash.slice(1));
    if (!target) return false;
    document.querySelectorAll('.comment.is-target').forEach(function (comment) { comment.classList.remove('is-target'); });
    target.classList.add('is-target');
    target.focus({ preventScroll: true });
    target.scrollIntoView({ behavior: 'smooth', block: 'center' });
    return true;
  }

  async function loadComments(offset, append) {
    if (commentsLoading) return;
    commentsLoading = true;
    status.hidden = false;
    status.textContent = offset ? t('pagination.loading') : t('comments.loading');
    if (more) more.disabled = true;
    try {
      await (window.gopressCurrentUser || Promise.resolve(null));
      var response = await fetch(endpoint + '?offset=' + offset, { headers: window.gopressAuthHeaders({ Accept: 'application/json' }) });
      if (!response.ok) throw new Error(t('comments.load_failed'));
      var payload = await response.json();
      var items = payload && Array.isArray(payload.items) ? payload.items : [];
      loadedComments = append ? loadedComments.concat(items) : items;
      nextOffset = Number(payload.nextOffset) || 0;
      hasMore = Boolean(payload.hasMore);
      if (append) appendComments(items);
      else renderComments(loadedComments);
      if (more) {
        more.hidden = !hasMore;
        more.disabled = false;
      }
      focusHashTarget();
    } catch (error) {
      if (!append) list.replaceChildren();
      status.textContent = error.message;
      if (more) more.disabled = false;
    } finally {
      commentsLoading = false;
    }
  }

  async function loadHashTarget() {
    if (focusHashTarget()) return;
    if (!window.location.hash.startsWith('#comment-')) {
      await loadComments(0, false);
      return;
    }
    var fragment = window.location.hash.slice('#comment-'.length);
    try {
      var response = await fetch('/v1/comments/location/' + encodeURIComponent(fragment), {
        headers: window.gopressAuthHeaders({ Accept: 'application/json' })
      });
      if (!response.ok) throw new Error(t('comments.not_found'));
      var location = await response.json();
      await loadComments(Number(location.offset) || 0, false);
      focusHashTarget();
    } catch (error) {
      status.hidden = false;
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
      message.setAttribute('role', 'alert');
      message.classList.remove('is-success');
      submit.disabled = true;

      try {
        await (window.gopressCurrentUser || Promise.resolve(null));
        var response = await fetch(endpoint, {
          method: 'POST',
          headers: window.gopressAuthHeaders({
            Accept: 'application/json',
            'Content-Type': 'application/json'
          }),
          body: JSON.stringify({
            parentId: parentId || '',
            name: String(data.get('name') || '').trim(),
            email: String(data.get('email') || '').trim(),
            text: String(data.get('text') || '').trim()
          })
        });
        var payload = await response.json().catch(function () { return {}; });
        if (!response.ok) {
          throw new Error(payload.error && payload.error.description ? payload.error.description : t('comments.publish_failed'));
        }

        if (!parentId) {
          form.elements.text.value = '';
          message.textContent = t('comments.published');
          message.setAttribute('role', 'status');
          message.classList.add('is-success');
        } else {
          form.remove();
          openReply = null;
        }
        var fragment = '';
        try { fragment = new URL(payload.url, window.location.href).hash; } catch (_) {}
        if (fragment) history.replaceState(null, '', window.location.pathname + window.location.search + fragment);
        await loadHashTarget();
      } catch (error) {
        message.textContent = error.message;
      } finally {
        submit.disabled = false;
      }
    });
  }

  bindForm(rootForm, '');
  if (more) more.addEventListener('click', function () { loadComments(nextOffset, true); });
  window.addEventListener('hashchange', function () {
    if (!focusHashTarget()) loadHashTarget();
  });
  if (window.location.hash.startsWith('#comment-')) loadHashTarget();
  else loadComments(0, false);
})();
