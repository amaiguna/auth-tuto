const API = 'http://localhost:3000'
const app = document.getElementById('app')

// CSRF トークンはセッション中だけ有効な秘密値。XSS 経由で抜かれないよう
// localStorage に入れず、モジュールスコープのメモリ変数で保持する。
// ページリロードや再ログインのたびに /csrf-token から取り直す。
let csrfToken = null

async function route() {
  if (window.location.pathname === '/loggedout') {
    renderLoggedOut()
    return
  }
  const me = await fetchMe()
  if (!me) {
    renderAnonymous()
    return
  }
  csrfToken = await fetchCsrfToken()
  renderLoggedIn(me)
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

async function fetchCsrfToken() {
  const res = await fetch(`${API}/csrf-token`, { credentials: 'include' })
  if (!res.ok) return null
  const data = await res.json()
  return data.csrf_token
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

async function logout() {
  // form 送信ではカスタムヘッダを付けられないので fetch に変更。
  // サーバーは Keycloak end_session の URL を JSON で返してくるので、
  // それを受け取ってから location.href で top-level 遷移を行う。
  const res = await fetch(`${API}/logout`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'X-CSRF-Token': csrfToken },
  })
  if (!res.ok) {
    console.error('logout failed', res.status)
    return
  }
  const { logout_url } = await res.json()
  window.location.href = logout_url
}

route()
