const API = 'http://localhost:3000'
const app = document.getElementById('app')

async function route() {
  if (window.location.pathname === '/loggedout') {
    renderLoggedOut()
    return
  }
  const me = await fetchMe()
  if (me) renderLoggedIn(me)
  else renderAnonymous()
}

async function fetchMe() {
  try {
    const res = await fetch(`${API}/me`, { credentials: 'include' })
    if (!res.ok) return null
    return await res.json()
  } catch {
    return null
  }
}

function renderLoggedIn(me) {
  app.innerHTML = `
    <h1>auth-tuto</h1>
    <div class="card loggedin">
      <p>ログイン中: <strong data-name></strong></p>
      <button id="logout">ログアウト</button>
      <span class="muted"><a href="${API}/me" target="_blank">/me (raw)</a></span>
    </div>
  `
  app.querySelector('[data-name]').textContent = me.name
  document.getElementById('logout').addEventListener('click', logout)
}

function renderAnonymous() {
  app.innerHTML = `
    <h1>auth-tuto</h1>
    <div class="card loggedout">
      <p>未ログイン</p>
      <a href="${API}/login" class="button">ログイン</a>
    </div>
  `
}

function renderLoggedOut() {
  app.innerHTML = `
    <h1>ログアウトしました</h1>
    <a href="/" class="button">トップへ</a>
  `
}

function logout() {
  // POST + redirect chain (Keycloak end_session → :5173/loggedout) を辿らせるため form 送信で遷移させる
  const form = document.createElement('form')
  form.method = 'POST'
  form.action = `${API}/logout`
  document.body.appendChild(form)
  form.submit()
}

route()
