(function () {
  'use strict';

  var login = document.getElementById('login');
  var logout = document.getElementById('logout');

  function showAnonymous() {
    if (login) login.hidden = false;
    if (logout) logout.hidden = true;
  }

  function rememberLoginReturn(target) {
    try {
      window.sessionStorage.setItem('gopressLoginReturn', target);
    } catch (error) {
      // Login still works when storage is unavailable, without the return shortcut.
    }
  }

  window.gopressRequireLogin = function (target) {
    rememberLoginReturn(target);
    window.location.href = '/auth/login?redirect=' + encodeURIComponent(target);
  };

  if (login) {
    var loginLink = login.querySelector('a[href="/auth/login"]');
    if (loginLink) {
      loginLink.addEventListener('click', function () {
        rememberLoginReturn(window.location.pathname + window.location.search + window.location.hash);
      });
    }
  }

  function resumeAfterLogin() {
    var target;
    try {
      target = window.sessionStorage.getItem('gopressLoginReturn');
      window.sessionStorage.removeItem('gopressLoginReturn');
    } catch (error) {
      return false;
    }
    if (!target) return false;

    var destination;
    try {
      destination = new URL(target, window.location.origin);
    } catch (error) {
      return false;
    }
    if (destination.origin !== window.location.origin || destination.href === window.location.href) return false;
    window.location.replace(destination.href);
    return true;
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

      if (resumeAfterLogin()) return me;
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
