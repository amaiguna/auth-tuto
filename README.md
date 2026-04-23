# auth-tuto

認証/認可の勉強を、実コードを書きながら進めるための個人リポジトリ。

## やりたいこと

- OIDC プロトコルによる JWT を用いた認証ロジックの理解と実装
- CSRF など、現代の Web アプリに必須となっている攻撃対策の実装と検証
  - HttpOnly / Secure / SameSite 属性の挙動確認
  - CSRF トークン
  - など

まずは **認証** から。認可はアプリロジックに踏み込む話なので後回し。

## 構成

フロントエンドとバックエンドを分離した 4 者構成：

```mermaid
flowchart LR
    Browser[Browser] -->|1. アクセス| FE[Frontend SPA<br/>Vite :5173]
    FE -->|2. fetch /me w/ Cookie| BE[Backend<br/>Go + Echo :3000]
    Browser -->|3. /login へ遷移| BE
    BE -->|4. authorize redirect| IdP[IdP<br/>Keycloak :8080]
    Browser -->|5. authn| IdP
    IdP -->|6. code| BE
    BE -->|7. token exchange| IdP
    BE -->|8. Set-Cookie + redirect| Browser
```

- **Frontend (SPA)**: Vanilla JS + Vite (`frontend/`)。UI と fetch を担当
- **Backend (RP)**: Go + Echo (`backend/`)。OIDC の中継、セッション管理、CSRF 対策のメイン実装先。JSON API
- **IdP (OP)**: Keycloak を Docker で起動
- **Resource Server**: 当初は Backend と同居。JWT 検証の勉強段階で分離を検討

Frontend と Backend はクロスオリジン (`:5173` ↔ `:3000`) にしている。`SameSite` / `Origin` / CORS などの挙動を現実的な形で学習するため。

全サービスを `docker compose up` 1 コマンドで起動できる。Backend は air によるホットリロード対応。

## 技術スタック

| レイヤー | 選定 |
| --- | --- |
| Backend 言語 | Go |
| Backend フレームワーク | [Echo](https://echo.labstack.com/) |
| Frontend | Vanilla JS + [Vite](https://vite.dev/) |
| IdP | Keycloak (Docker) |

フレームワーク（React 等）は入れない。CSRF / Cookie / fetch 周りの挙動を最小構成で追えることを優先。

## ドキュメント

実装フェーズ完了ごとに `docs/` 配下に学習まとめを追記していく。

| ファイル | 内容 |
|---|---|
| [docs/01-oidc-auth-code-flow.md](docs/01-oidc-auth-code-flow.md) | OIDC Authorization Code Flow、JWT 構造と検証クレーム |
| [docs/02-session-cookie.md](docs/02-session-cookie.md) | セッション管理、Cookie の仕組みと HttpOnly 属性 |
| [docs/03-csrf-problems.md](docs/03-csrf-problems.md) | CSRF とは何か、典型的な被害、Login CSRF |

## 学習ロードマップ

走りながら決めるので前後・分岐あり。現状の想定順：

### 認証の基礎（進行中）

- [x] Keycloak を Docker で立てる（realm JSON で宣言的に）
- [x] `/login` → Keycloak authorization endpoint へ redirect
- [x] `/callback` で code を token exchange
- [x] id_token の payload をパースしてユーザー識別
- [x] session をサーバー側（インメモリ map）で管理、session_id を HttpOnly Cookie で保持
- [x] `/me` で Cookie からユーザー情報を返す
- [x] `/logout` で RP セッション削除 + Keycloak `end_session_endpoint` で IdP 側セッション終了
- [x] `state` パラメータで Login CSRF 対策

### CSRF 対策（次ここから）

- [x] 問題の整理（docs/03）
- [ ] SameSite 属性による防御と挙動確認
- [ ] CSRF トークンによる明示的な対策（二重送信 Cookie パターンなど）
- [ ] `/logout` 含む状態変更エンドポイントに CSRF トークン適用

### JWT の扱い

- [ ] id_token の署名検証（JWKS から公開鍵取得 → RS256 検証）
- [ ] `iss` / `aud` / `exp` / `nonce` のクレーム検証
- [ ] `nonce` パラメータ実装（replay 対策）
- [ ] access_token の検証（Resource Server 分離時）

### Resource Server 分離

- [ ] 別プロセスとして Resource Server を立てる
- [ ] `access_token` を Authorization ヘッダで受け取り検証する API

### その他（優先度は都度判断）

- [x] フロントエンドを SPA として分離（Vite + vanilla JS）
- [x] 全サービスを docker compose に統合、backend を `backend/` へ移動
- [x] air によるバックエンドホットリロード
- [x] Keycloak URL を env var で分離（内部通信 vs ブラウザ向け redirect）
- [ ] 設定値の環境変数化（今は main.go に直書き）
- [ ] セッション map を `sync.Map` に置き換える
- [ ] PKCE（SPA から IdP に直接アクセスする形態の勉強時）

### 認可（ずっと先）

認証が固まってから着手。RBAC / ABAC / scope ベースなどを予定。

## 起動方法

```sh
docker compose up
```

`http://localhost:5173/` へアクセス。

- コード変更は air (backend) / Vite HMR (frontend) により自動リロード
- Keycloak の realm 変更は `docker compose restart keycloak` が必要
- 初回は backend イメージのビルドに時間がかかる (`docker compose build backend`)

## 進め方

- 機能単位で一歩ずつ、動作確認しながら進める。
- 大きな設計を先に固めず、走りながら決める。
- ミドルウェアで一発解決せず、まず自分で書いて挙動を理解してから標準実装に置き換え、差分を読む。
