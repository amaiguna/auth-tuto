# 02. セッション管理と Cookie

## なぜ token をそのままブラウザに渡してはいけないか

token（id_token / access_token）をブラウザに直接渡すと2つの問題がある。

1. **XSS で盗まれると手が打てない**  
   token は Keycloak が発行した証明書なので、RP 側から無効化できない。有効期限まで悪用され続ける。

2. **localStorage は JS から丸読みされる**  
   XSS が起きた瞬間に token が抜かれる。

### 正しい設計

```mermaid
sequenceDiagram
    participant Browser
    participant RP as RP (Echo)

    RP->>RP: token を受け取り、サーバー内に保管
    RP->>RP: session ID (UUID) を生成
    RP->>Browser: Set-Cookie: session_id=<uuid>; HttpOnly
    Browser->>RP: GET /me（Cookie: session_id=<uuid> が自動で付く）
    RP->>RP: sessions[uuid] からユーザー情報を取り出す
    RP->>Browser: ユーザー情報を返す
```

- token はサーバーが持つ。ブラウザには渡さない
- ブラウザには **意味のない session ID だけ** を渡す
- session ID が盗まれてもサーバー側で削除すれば即無効にできる

---

## Cookie とは

サーバーがブラウザに「これ覚えといて」と渡すキーバリュー。

```
サーバー → ブラウザ: Set-Cookie: session_id=abc123
ブラウザ → サーバー: Cookie: session_id=abc123  （以降のリクエストに自動で付く）
```

**「自動で付く」** のが最大の特徴。JS で明示的に送らなくても同じドメインへのリクエストに乗ってくる。これは便利な反面、CSRF の原因にもなる（後のフェーズで対策する）。

### 主な属性

| 属性 | 意味 |
|---|---|
| `HttpOnly` | JS から `document.cookie` で読めなくなる。XSS 対策の核心 |
| `Secure` | HTTPS のときだけ送信される |
| `SameSite` | 別ドメインからのリクエストに Cookie を付けるか制御。CSRF 対策 |
| `Path` | どのパスへのリクエストに付けるか |
| `Expires` / `Max-Age` | 有効期限 |

### Cookie vs localStorage

| | HttpOnly Cookie | localStorage |
|---|---|---|
| JS から読めるか | 読めない | 読める（XSS で盗まれる） |
| リクエストに自動で付くか | 付く | 付かない（JS で明示的に送る必要あり） |
| CSRF のリスク | ある（自動で付くゆえに） | ない |

---

## セッション管理の実装

### インメモリ session map

```go
var sessions = map[string]idTokenPayload{}
```

パッケージレベルで宣言。本来はスレッドセーフな `sync.Map` を使うべきだが、学習段階では素の `map` で十分。

### /callback の流れ

1. code → token exchange
2. `id_token` の payload（JWT の第2パート）を Base64 デコード
3. `sub` と `preferred_username` を取り出す
4. `uuid.NewString()` で session ID を生成
5. `sessions[id] = payload` で保存
6. `Set-Cookie: session_id=<id>; HttpOnly; Path=/` をセット
7. `/me` へ redirect

```go
cookie := new(http.Cookie)
cookie.Name = "session_id"
cookie.Value = id
cookie.HttpOnly = true
cookie.Path = "/"
c.SetCookie(cookie)
```

### /me の流れ

```go
sessionCookie, err := c.Cookie("session_id")
if err != nil {
    return c.NoContent(http.StatusUnauthorized)
}

payload, ok := sessions[sessionCookie.Value]
if !ok {
    return c.NoContent(http.StatusUnauthorized)
}
```

- Cookie がない → 401
- Cookie はあるが session map に存在しない → 401（無効な session ID）
- 存在する → ユーザー情報を返す

map からの取り出しは第二戻り値 `ok` で存在確認するのが Go の慣用句。存在しないキーはゼロ値を返すため `ok` なしでは判定できない。

---

## JWT の Base64 デコード

JWT は `.` 区切りの3パート構成。payload は第2パート。

```go
parts := strings.Split(tokens.IDToken, ".")
payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
```

`RawURLEncoding` を使う理由：
- `Raw` = `=` パディングなし
- `URL` = `+` `/` の代わりに `-` `_` を使う

JWT はこの形式（RFC 7515）で定められている。

---

## Go のメモ：defer の挙動

`defer` は宣言した行を**通過した時点で登録**され、関数 return 時に実行される。

```go
resp, err := http.PostForm(...)

if err != nil {
    return err  // ここで return → defer はまだ登録されていないので実行されない
}
defer resp.Body.Close()  // この行を通過して初めて登録される
```

err チェックより前に `defer resp.Body.Close()` を書くと、`resp` が `nil` のときに panic する。

---

## この時点のエンドポイント構成

| エンドポイント | 処理 |
|---|---|
| `GET /login` | Keycloak の authorization endpoint へ redirect |
| `GET /callback` | code → token exchange → session 生成 → Cookie セット → `/me` へ redirect |
| `GET /me` | Cookie の session_id からユーザー情報を返す |
