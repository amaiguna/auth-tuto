package main

import "os"

const (
	clientID              = "echo-app"
	clientSecret          = "supersecret"
	redirectURI           = "http://localhost:3000/callback"
	frontendOrigin        = "http://localhost:5173"
	frontendTopURL        = frontendOrigin + "/"
	postLogoutRedirectURI = frontendOrigin + "/loggedout"

	ctxKeySession   = "session"
	ctxKeySessionId = "session_id"
)

// ブラウザから辿る URL と、バックエンド→Keycloak のサーバー間通信に使う URL を分けている。
// docker compose で動かす時は Keycloak の内部 URL (http://keycloak:8080/...) を env で注入する。
var (
	keycloakAuthBase  = getenv("KEYCLOAK_AUTH_BASE", "http://localhost:8080/realms/auth-tuto")
	keycloakTokenBase = getenv("KEYCLOAK_TOKEN_BASE", "http://localhost:8080/realms/auth-tuto")
)

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
