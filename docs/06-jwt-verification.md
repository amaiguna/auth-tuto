# 06. JWT 署名検証 — 概念と実装

OIDC で受け取る ID Token (= JWT) を「本当に IdP が発行したものか」を確認する仕組み。docs/01〜05 では token を Keycloak から HTTPS で受け取って payload を base64 デコードしているだけ → JWT を「JSON コンテナ」としてしか扱っていなかった。本回から JWT を **「署名で身元保証された JSON」** として扱う段階に入る。

今回の実装では、Keycloak の JWKS から公開鍵を取得し、ID Token の RS256 署名を検証したうえで、`iss` / `aud` / `exp` クレームを確認するところまで進めた。`nonce` は次フェーズで扱う。

## 1. JWT の中身 — 3 パート構造

JWT は 3 パートを `.` で繋いだ文字列:

```
eyJhbGciOiJSUzI1NiIs...   .   eyJzdWIiOiI1NjMyM...   .   X3JhJyc4kI...
       header                       payload                  signature
```

| パート | 中身 (base64url decode 後) | 例 |
|---|---|---|
| header | アルゴリズムと **どの鍵で署名したか (`kid`)** | `{"alg":"RS256","kid":"abc123"}` |
| payload | クレーム (`sub`, `iss`, `aud`, `exp` など) | `{"sub":"alice","exp":1735689600,"aud":"echo-app"}` |
| signature | header と payload に対する電子署名 | バイナリを base64url したもの |

**header と payload は誰でも読める** — base64url は単なるエンコードであり暗号ではない。価値はすべて signature が **「IdP しか作れない」** ことに集約されている。

逆に言うと: **「signature を検証していない JWT」は payload を任意に書き換え可能なただの JSON とほぼ同じ**。docs/05 までの実装はこれに該当している。

## 2. 署名 = 公開鍵暗号 (非対称鍵)

ここが理解の中核。

```mermaid
flowchart LR
    subgraph IdP["Keycloak が持つ"]
        Priv[("秘密鍵<br/>private key")]
        Pub[("公開鍵<br/>public key")]
    end

    Priv -- "JWT を署名" --> SignedJWT["署名済み JWT"]
    SignedJWT --> Verifier["Backend / Resource Server"]
    Pub -- "公開鍵で検証" --> Verifier

    style Priv fill:#ffd
    style Pub fill:#dfd
```

| 鍵 | 持つ人 | やれること |
|---|---|---|
| **秘密鍵** | IdP (Keycloak) のみ | 署名を**作る** |
| **公開鍵** | 世界中の誰でも (公開してよい) | 署名を**検証**するだけ。**作れない** |

これが **非対称鍵暗号 (asymmetric cryptography)** の中核性質。`RS256` = RSA + SHA256 の意味。

OIDC は IdP と RP を分離する以上、**必ず非対称鍵を使う**。対称鍵 (`HS256` = HMAC) を使ってしまうと、検証側 (RP) も JWT を作れることになり「IdP が発行した」を保証できなくなる。

## 3. 「署名検証」は具体的に何をする操作か

```mermaid
sequenceDiagram
    participant BE as Backend
    participant JWT as 受け取った JWT

    BE->>JWT: header と payload を取り出す
    BE->>BE: "header.payload" の文字列を SHA256 でハッシュ
    BE->>JWT: signature を取り出す
    BE->>BE: 公開鍵で signature を「復号」 → ハッシュ値が出てくる
    Note over BE: 2 つのハッシュが一致するか?
    alt 一致
        BE-->>BE: 「Keycloak が署名した本物 + 改ざんなし」
    else 不一致
        BE-->>BE: 「改ざん or 偽物」→ reject
    end
```

ハッシュが一致することの意味:

- **`header.payload` 部分が 1 ビットも変わっていない** = 改ざんされていない
- **その signature は秘密鍵を持つ者にしか作れない** = IdP が作ったものだ

の 2 つを同時に証明する。これが RSA の数学的性質 (公開鍵で復号できるのは対応する秘密鍵で署名されたものだけ) に依存している。

## 4. 公開鍵をどう入手するか — JWKS

Backend が IdP の公開鍵を持っていないと検証できない。**ハードコードはダメ** (IdP は鍵をローテーションする)。OIDC は「IdP が公開鍵を JSON で配信する規格」を持つ — これが **JWKS (JSON Web Key Set)**。

Keycloak の場合、エンドポイントは:

```
/realms/auth-tuto/protocol/openid-connect/certs
```

レスポンス例 (簡略):

```json
{
  "keys": [
    {
      "kid": "abc123",
      "kty": "RSA",
      "alg": "RS256",
      "n": "0vx7agoebGcQSuuPiLJX...",
      "e": "AQAB"
    },
    { "kid": "old-key-456", "...": "..." }
  ]
}
```

| フィールド | 意味 |
|---|---|
| `kid` | Key ID。JWT header の `kid` と突き合わせる索引 |
| `kty` | Key Type。`RSA` か `EC` |
| `alg` | 想定アルゴリズム (`RS256` など) |
| `n`, `e` | RSA 公開鍵の構成要素 (modulus と exponent)。これから `*rsa.PublicKey` を組み立てる |

### なぜ複数キーを返すのか — 鍵ローテーション

```mermaid
flowchart LR
    T1["時刻 T<br/>新キー abc123 で署名開始"]
    T2["時刻 T+1日<br/>JWKS に abc123 と old-key-456 両方"]
    T3["時刻 T+30日<br/>JWKS から old-key-456 削除"]
    T1 --> T2 --> T3
```

鍵をローテーションしても、**過渡期に古い鍵で署名された JWT がまだ有効期限内** であることがある。新旧両方を一定期間配信することで両方検証可能な状態を保つ。これが JWKS が単一鍵ではなく **配列** になっている理由。

## 5. 全体の流れ (Backend 視点)

```mermaid
sequenceDiagram
    participant U as User
    participant BE as Backend (RP)
    participant KC as Keycloak (IdP)

    Note over BE: 起動時 or 定期的に
    BE->>KC: GET /.../certs
    KC-->>BE: JWKS (公開鍵リスト)
    Note over BE: 公開鍵を kid で索引化してキャッシュ

    Note over U,KC: --- ユーザーがログイン ---
    U->>BE: /callback?code=...
    BE->>KC: code → token 交換
    KC-->>BE: id_token (JWT)

    Note over BE: ここから検証
    BE->>BE: JWT header から kid を読む
    BE->>BE: キャッシュから kid に対応する公開鍵を引く
    BE->>BE: 公開鍵で署名検証
    BE->>BE: payload のクレーム検証 (iss/aud/exp)
    Note over BE: 全部 OK ならセッション化
```

## 6. なぜ HTTPS だけでは不十分なのか

「Keycloak から HTTPS で取ってきたんだから、安全じゃないの?」という問いに先回りして答える。

HTTPS が保証するのは:

- 通信路の**盗聴防止** (途中で覗かれない)
- 通信路の**改ざん防止** (途中で書き換えられない)
- 通信相手の**サーバ認証** (TLS 証明書で「確かに keycloak.example.com と話している」)

つまり HTTPS は **「2 点間の通信」だけ** を守る。

JWT 署名検証が解くのは別の脅威:

| 脅威 | HTTPS で防げるか | JWT 署名で防げるか |
|---|---|---|
| 通信途中で MITM が JWT を書き換える | ✅ | ✅ |
| 攻撃者が偽の JWT を URL fragment / Cookie / postMessage で持ち込む | ❌ | ✅ |
| JWT を別のサービスから流用される (audience 混同) | ❌ | ✅ (`aud` クレーム検証) |
| JWT を盗んで有効期限切れ後に使う | ❌ | ✅ (`exp` クレーム検証) |
| Resource Server が「自分宛じゃない IdP」の JWT を信じてしまう | ❌ | ✅ (`iss` クレーム検証) |

特に Resource Server を分離した瞬間、JWT は Authorization ヘッダで持ち運ばれて様々な経路を経由する → **JWT 自体が単独で身元保証できないと話にならない**。

現状のリポジトリ構成 (token を Keycloak から直接受け取り、Backend のメモリに留めるだけ) でも、**理屈上は `handleCallback` は「payload を信じて良い理由」を持っていない**。HTTPS が「Keycloak と話している」を保証してくれている恩恵で結果的に安全になっているだけで、本来の OIDC 仕様としては署名検証が必須。

## 7. 周辺の補足

### `crypto/rsa` での「公開鍵で復号」って何

正確には RSA の検証は「復号」ではなく **「signature を公開鍵で変換するとパディング付きハッシュ値が得られる、それを検算」** する操作。Go では `rsa.VerifyPKCS1v15(pub, crypto.SHA256, hashed, sig)` 一発で済む。手書きする部分はこの呼び出しの **前後の準備**:

1. `header.payload` 文字列を SHA256 でハッシュ
2. JWKS の `n` (modulus), `e` (exponent) から `*rsa.PublicKey` を組み立て
3. signature を base64url decode してバイト列に戻す
4. `VerifyPKCS1v15` を呼んでエラー無しなら検証成功

### JWKS のキャッシュ TTL

定石は **数分 〜 1 時間**。短すぎると IdP に負荷、長すぎると鍵ローテーション直後に新キーの JWT を弾く時間が伸びる。Keycloak のデフォルトは Cache-Control ヘッダで `max-age=600` (10 分) を返してくる。今回の実装では単純化のため callback ごとに JWKS を取得しており、後段で TTL キャッシュ化を検討する。

### `kid` がない / アルゴリズムを `none` に偽装する攻撃

JWT 検証の有名な脆弱性パターン:

- **`alg: none` 攻撃**: header に「アルゴリズムなし」と書かれた JWT を受理させて signature を素通しさせる
- **`alg: HS256` 偽装攻撃**: RS256 想定のサーバに HS256 と書いた JWT を投げる。HMAC は対称鍵なので、公開鍵を共有鍵として使って攻撃者が署名を作れてしまう (公開鍵は文字通り公開されている)

実装時に意識する原則: **「JWT 側の `alg` を信じて分岐するのではなく、検証側で受け入れる `alg` を `RS256` に固定する」**。`alg` フィールドはあくまで参考情報として読むだけ。

### ID Token vs Access Token

OIDC で受け取る token は 2 種類 (+ refresh):

| | 用途 | 形式 | 検証する人 |
|---|---|---|---|
| **ID Token** | 「誰がログインしたか」を RP に伝える | 必ず JWT | RP (Backend) |
| **Access Token** | API 呼び出しの認可情報 | JWT のことが多いが規格上は不透明 (opaque) でもよい | Resource Server |

本リポジトリ Sub A〜C では **ID Token** の検証を扱う。Access Token の検証は Resource Server 分離フェーズで別途扱う (構造は同じ)。

## 8. 今回の実装

今回の実装では、`/callback` で token exchange 後に受け取った `id_token` を、セッション化する前に検証するようにした。

```mermaid
sequenceDiagram
    participant Browser
    participant BE as Backend
    participant KC as Keycloak

    Browser->>BE: GET /callback?code=...&state=...
    BE->>BE: state Cookie と query state を照合
    BE->>KC: POST /token
    KC-->>BE: id_token
    BE->>BE: JWT header から alg / kid を読む
    BE->>KC: GET /certs
    KC-->>BE: JWKS
    BE->>BE: kid に一致する JWK を探す
    BE->>BE: n/e から RSA 公開鍵を復元
    BE->>BE: RS256 署名検証
    BE->>BE: iss / aud / exp 検証
    BE->>BE: 検証済み claims から sessionData を作成
    BE-->>Browser: Set-Cookie: session_id=...
```

実装後の `handleCallback` は、HTTP ハンドラとして必要な流れだけを読む形に整理した。

```go
func handleCallback(c echo.Context) error {
    if err := validateOAuthState(c); err != nil {
        return c.NoContent(http.StatusBadRequest)
    }

    tokens, err := exchangeCodeForTokens(c.QueryParam("code"))
    if err != nil {
        return c.NoContent(http.StatusBadGateway)
    }

    claims, err := verifyIDToken(tokens.IDToken)
    if err != nil {
        if errors.Is(err, errOIDCUpstream) {
            return c.NoContent(http.StatusBadGateway)
        }
        return c.NoContent(http.StatusUnauthorized)
    }

    sd := newSessionData(claims, tokens.IDToken)
    id := uuid.NewString()

    sessions[id] = sd
    setSessionCookie(c, id)

    return c.Redirect(http.StatusFound, frontendTopURL)
}
```

### ファイル分割

`main.go` にすべてを書き続けると、HTTP の流れ、OIDC、JWT、セッション、CSRF が混ざって見通しが悪くなるため、`package main` のままファイルだけ分割した。

| ファイル | 役割 |
|---|---|
| `main.go` | Echo 起動、CORS、ルーティング |
| `config.go` | client ID、redirect URI、Keycloak URL、context key |
| `types.go` | `tokenResponse`、`idTokenClaims`、`jwtHeader`、`jwks`、`sessionData` |
| `handlers.go` | `/login`、`/callback`、`/logout`、`/me`、`/csrf-token` |
| `oidc.go` | token exchange、ID Token 検証、JWKS 取得、署名検証、claim 検証 |
| `middleware.go` | `RequireSession`、`RequireCSRF` |
| `session.go` | インメモリ session map |
| `errors.go` | OIDC / OAuth state 周辺の内部エラー |

パッケージは分けていない。学習段階では、ドメイン境界を強く切るよりも、同じ `main` パッケージ内で責務ごとにファイルを分ける方が読みやすい。

### ID Token 検証の分割

`verifyIDToken` は、JWT 検証の手順が上から読めるようにした。

```go
func verifyIDToken(idToken string) (idTokenClaims, error) {
    parts := strings.Split(idToken, ".")
    if len(parts) != 3 {
        return idTokenClaims{}, errInvalidIDToken
    }

    header, err := parseJWTHeader(parts[0])
    claims, err := parseIDTokenClaims(parts[1])
    signatureBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
    keySet, err := fetchJWKS()
    matchedKey, err := findJWKByKid(keySet, header.Kid)
    publicKey, err := rsaPublicKeyFromJWK(*matchedKey)

    if err := verifyJWTSignature(parts, publicKey, signatureBytes); err != nil {
        return idTokenClaims{}, err
    }

    if err := validateIDTokenClaims(claims); err != nil {
        return idTokenClaims{}, err
    }

    return claims, nil
}
```

上のコードは流れを示す抜粋。実際のコードでは各 `err` を都度チェックしている。

分割した関数の責務:

| 関数 | 役割 |
|---|---|
| `exchangeCodeForTokens` | authorization code を Keycloak token endpoint へ送り、token response を得る |
| `parseJWTHeader` | JWT header を base64url decode し、`alg == RS256` と `kid` の存在を確認 |
| `parseIDTokenClaims` | payload を `idTokenClaims` に bind |
| `fetchJWKS` | Keycloak の `/certs` から JWKS を取得 |
| `findJWKByKid` | JWT header の `kid` と一致する JWK を探す |
| `rsaPublicKeyFromJWK` | JWK の `n` / `e` から `*rsa.PublicKey` を作る |
| `verifyJWTSignature` | `header.payload` の SHA-256 hash と signature を RS256 で検証 |
| `validateIDTokenClaims` | `iss` / `aud` / `exp` を検証 |

### claims と sessionData を分ける

当初は payload の内容を `sessionData` に直接 bind していたが、今回 `idTokenClaims` と `sessionData` を分けた。

```go
type idTokenClaims struct {
    Sub               string `json:"sub"`
    Iss               string `json:"iss"`
    Aud               string `json:"aud"`
    Exp               int64  `json:"exp"`
    PreferredUsername string `json:"preferred_username"`
}

type sessionData struct {
    Sub               string `json:"sub"`
    PreferredUsername string `json:"preferred_username"`
    IDToken           string `json:"id_token"`
    CSRFToken         string `json:"-"`
}
```

`idTokenClaims` は **外部から来た ID Token の主張**。`sessionData` は **検証後に backend が保持する内部状態**。`iss` / `aud` / `exp` は検証には必要だが、ログイン後の `/me` や `/logout` で直接使わないため session には保存しない。

`exp` は JWT 上では Unix timestamp の数値なので、`time.Time` ではなく `int64` として受ける。

```go
if claims.Exp <= time.Now().Unix() {
    return errInvalidIDToken
}
```

### 署名検証

署名対象は、JWT の `header.payload` 部分そのもの。

```go
signingInput := parts[0] + "." + parts[1]
hashed := sha256.Sum256([]byte(signingInput))

err := rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, hashed[:], signatureBytes)
```

`sha256.Sum256` は `[32]byte` を返す。一方 `VerifyPKCS1v15` は `[]byte` を要求するため、`hashed[:]` で配列全体をスライスとして渡している。

### 今回まだやっていないこと

- JWKS のキャッシュ。現状は callback ごとに `/certs` を取得する
- `aud` が配列の場合の対応。現状は `string` 前提
- `nonce` による replay 対策
- `iat` / `nbf` など追加クレームの検証
- 標準ライブラリや JWT ライブラリへの置き換え

## 次のフェーズ

ID Token の署名検証と最小限の claim 検証は実装済み。次は `nonce` パラメータを追加し、login request と ID Token の対応を検証する。

その先:

- **Resource Server 分離 + access_token 検証** (docs/07 候補)
- (発展) 標準ライブラリ (`go-jose`, `lestrrat-go/jwx`, `golang-jwt`) への置き換えと差分観察
