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
})();
