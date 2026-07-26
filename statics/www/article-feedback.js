(function () {
  'use strict';

  var section = document.getElementById('feedback');
  var t = (window.gopressI18n && window.gopressI18n.t) || function (key) { return key; };
  if (!section) return;

  var endpoint = section.dataset.endpoint;
  var title = section.dataset.title;
  var targetUrl = new URL(section.dataset.url || window.location.href.split('#')[0], window.location.href).href;
  var likeButton = document.getElementById('article-like');
  var bookmarkButton = document.getElementById('article-bookmark');
  var likeCount = document.getElementById('article-like-count');
  var feedbackStatus = document.getElementById('feedback-status');
  var shareButton = document.getElementById('article-share');
  var shareDialog = document.getElementById('share-dialog');
  var shareStatus = document.getElementById('share-dialog-status');

  function renderFeedback(feedback) {
    likeCount.textContent = String(feedback.likes || 0);
    setPressed(likeButton, Boolean(feedback.liked), t('feedback.applaud'), t('feedback.remove_applause'));
    setPressed(bookmarkButton, Boolean(feedback.bookmarked), t('feedback.save'), t('feedback.remove_saved'));
    likeButton.disabled = false;
    bookmarkButton.disabled = false;
  }

  function setPressed(button, active, inactiveLabel, activeLabel) {
    button.dataset.active = active ? 'true' : 'false';
    button.setAttribute('aria-pressed', active ? 'true' : 'false');
    button.querySelector('.feedback-label').textContent = active ? activeLabel : inactiveLabel;
  }

  function loginReturnTarget() {
    return window.location.pathname + window.location.search + '#feedback';
  }

  async function requireUser() {
    var me = window.gopressCurrentUser ? await window.gopressCurrentUser : null;
    if (me && me.id) return true;
    window.gopressRequireLogin(loginReturnTarget());
    return false;
  }

  async function updateInteraction(button, resource) {
    if (!await requireUser()) return;

    button.disabled = true;
    feedbackStatus.textContent = '';
    try {
      var active = button.dataset.active === 'true';
      var response = await fetch(endpoint + '/' + resource, {
        method: active ? 'DELETE' : 'PUT',
        headers: window.gopressAuthHeaders({ Accept: 'application/json' })
      });
      if (response.status === 401) {
        window.gopressRequireLogin(loginReturnTarget());
        return;
      }
      var payload = await response.json().catch(function () { return {}; });
      if (!response.ok) {
        throw new Error(payload.error && payload.error.description ? payload.error.description : t('feedback.update_failed'));
      }
      renderFeedback(payload);
      feedbackStatus.textContent = resource === 'like'
        ? (payload.liked ? t('feedback.applause_added') : t('feedback.applause_removed'))
        : (payload.bookmarked ? t('feedback.saved_added') : t('feedback.saved_removed'));
    } catch (error) {
      feedbackStatus.textContent = error.message;
    } finally {
      button.disabled = false;
    }
  }

  async function copyLink(network) {
    try {
      await window.gopressCopyShareUrl(targetUrl);
      shareStatus.textContent = network
        ? t('feedback.link_copied_for').replace('{network}', network)
        : t('feedback.link_copied');
    } catch (error) {
      shareStatus.textContent = t('feedback.copy_failed');
    }
  }

  function shareUrl(network) {
    var url = targetUrl;
    var encodedUrl = encodeURIComponent(url);
    var encodedTitle = encodeURIComponent(title);
    var encodedMessage = encodeURIComponent(title + '\n' + url);
    var destinations = {
      bluesky: 'https://bsky.app/intent/compose?text=' + encodedMessage,
      threads: 'https://www.threads.net/intent/post?text=' + encodedMessage,
      linkedin: 'https://www.linkedin.com/sharing/share-offsite/?url=' + encodedUrl,
      x: 'https://twitter.com/intent/tweet?text=' + encodedTitle + '&url=' + encodedUrl,
      facebook: 'https://www.facebook.com/sharer/sharer.php?u=' + encodedUrl,
      whatsapp: 'https://wa.me/?text=' + encodedMessage,
      email: 'mailto:?subject=' + encodedTitle + '&body=' + encodedMessage
    };
    return destinations[network];
  }

  likeButton.addEventListener('click', function () {
    updateInteraction(likeButton, 'like');
  });
  bookmarkButton.addEventListener('click', function () {
    updateInteraction(bookmarkButton, 'bookmark');
  });

  shareButton.addEventListener('click', function () {
    shareStatus.textContent = '';
    if (typeof shareDialog.showModal === 'function') {
      shareDialog.showModal();
    } else {
      shareDialog.setAttribute('open', '');
    }
  });

  shareDialog.querySelector('.share-dialog-close').addEventListener('click', function (event) {
    if (typeof shareDialog.close === 'function') return;
    event.preventDefault();
    shareDialog.removeAttribute('open');
  });

  shareDialog.addEventListener('click', function (event) {
    if (event.target !== shareDialog) return;
    if (typeof shareDialog.close === 'function') {
      shareDialog.close();
    } else {
      shareDialog.removeAttribute('open');
    }
  });

  shareDialog.querySelectorAll('[data-share]').forEach(function (button) {
    button.addEventListener('click', function () {
      var network = button.dataset.share;
      if (network === 'copy') {
        copyLink('');
        return;
      }
      if (network === 'instagram' || network === 'youtube') {
        copyLink(network === 'instagram' ? 'Instagram' : 'YouTube');
        return;
      }
      window.open(shareUrl(network), '_blank', 'noopener,noreferrer');
    });
  });

  (window.gopressCurrentUser || Promise.resolve(null)).then(function () {
    return fetch(endpoint + '/feedback', {
      headers: window.gopressAuthHeaders({ Accept: 'application/json' })
    });
  })
    .then(function (response) {
      if (!response.ok) throw new Error(t('feedback.load_failed'));
      return response.json();
    })
    .then(renderFeedback)
    .catch(function (error) {
      likeButton.disabled = false;
      bookmarkButton.disabled = false;
      feedbackStatus.textContent = error.message;
    });
})();
