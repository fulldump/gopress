(function () {
  'use strict';

  var login = document.getElementById('login');
  var logout = document.getElementById('logout');
  if (!login || !logout) return;

  function showAnonymous() {
    login.hidden = false;
    logout.hidden = true;
  }

  fetch('/auth/me', { headers: { Accept: 'application/json' } })
    .then(function (response) {
      if (!response.ok) throw new Error('No session');
      return response.json();
    })
    .then(function (me) {
      if (me.error || !me.nick) {
        showAnonymous();
        return;
      }

      login.hidden = true;
      logout.hidden = false;
      document.getElementById('user_nick').textContent = me.nick;

      var picture = document.getElementById('user_picture');
      if (me.picture) {
        picture.src = me.picture;
        picture.alt = 'Foto de ' + me.nick;
        picture.hidden = false;
      }
    })
    .catch(showAnonymous);
})();
