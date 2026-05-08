package main

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

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

type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
	Kid string `json:"kid"`
}

type jwks struct {
	Keys []jwk `json:"keys"`
}

type jwk struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	IDToken      string `json:"id_token"`
	RefreshToken string `json:"refresh_token"`
}

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

// HACK: スレッドセーフでないのでsync.Mapに書き換え(必要なら)
var sessions = map[string]sessionData{}

func main() {
	e := echo.New()
	e.Use(middleware.RequestLogger())
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:     []string{frontendOrigin},
		AllowCredentials: true,
		AllowMethods:     []string{http.MethodGet, http.MethodPost},
	}))

	e.GET("/login", handleLogin)
	e.POST("/logout", handleLogout, RequireSession, RequireCSRF)
	e.GET("/callback", handleCallback)
	e.GET("/csrf-token", handleCSRF, RequireSession)
	e.GET("/me", handleMe, RequireSession)

	e.Start(":3000")
}

func handleLogin(c echo.Context) error {
	state := uuid.NewString()

	cookie := &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		HttpOnly: true,
		Path:     "/",
		SameSite: http.SameSiteLaxMode,
	}
	c.SetCookie(cookie)

	authURL, _ := url.Parse(keycloakAuthBase + "/protocol/openid-connect/auth")

	q := authURL.Query()
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("response_type", "code")
	q.Set("scope", "openid")
	q.Set("state", state)
	authURL.RawQuery = q.Encode()
	return c.Redirect(http.StatusFound, authURL.String())
}

func handleLogout(c echo.Context) error {

	sd := c.Get(ctxKeySession).(sessionData)

	logoutURL, _ := url.Parse(keycloakAuthBase + "/protocol/openid-connect/logout")
	q := logoutURL.Query()
	q.Set("id_token_hint", sd.IDToken)
	q.Set("post_logout_redirect_uri", postLogoutRedirectURI)

	logoutURL.RawQuery = q.Encode()

	sessionID := c.Get(ctxKeySessionId).(string)
	delete(sessions, sessionID)

	cookie := &http.Cookie{
		Name:     "session_id",
		Value:    "",
		MaxAge:   -1,
		Path:     "/",
		SameSite: http.SameSiteLaxMode,
	}

	c.SetCookie(cookie)

	return c.JSON(http.StatusOK, map[string]any{
		"logout_url": logoutURL.String(),
	})

}

func handleCallback(c echo.Context) error {

	stateFromQuery := c.QueryParam("state")
	stateCookie, err := c.Cookie("oauth_state")
	if err != nil || stateCookie.Value != stateFromQuery {
		return c.NoContent(http.StatusBadRequest)
	}

	c.SetCookie(&http.Cookie{
		Name:   "oauth_state",
		MaxAge: -1,
		Path:   "/",
	})

	code := c.QueryParam("code")

	tokenURL, _ := url.Parse(keycloakTokenBase + "/protocol/openid-connect/token")

	resp, err := http.PostForm(tokenURL.String(), url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
	})

	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.NoContent(http.StatusBadGateway)
	}

	var tokens tokenResponse
	err = json.NewDecoder(resp.Body).Decode(&tokens)
	if err != nil {
		return err
	}

	parts := strings.Split(tokens.IDToken, ".")
	if len(parts) != 3 {
		return c.NoContent(http.StatusBadRequest)
	}

	var header jwtHeader
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return err
	}

	err = json.Unmarshal(headerBytes, &header)
	if err != nil {
		return err
	}

	if header.Alg != "RS256" {
		return c.NoContent(http.StatusBadRequest)
	}

	if header.Kid == "" {
		return c.NoContent(http.StatusBadRequest)
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return err
	}

	var claims idTokenClaims
	err = json.Unmarshal(payloadBytes, &claims)
	if err != nil {
		return err
	}

	signatureBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return err
	}

	publicKeyURL, _ := url.Parse(keycloakTokenBase + "/protocol/openid-connect/certs")

	jwksResp, err := http.Get(publicKeyURL.String())
	if err != nil {
		return err
	}
	defer jwksResp.Body.Close()

	if jwksResp.StatusCode != http.StatusOK {
		return c.NoContent(http.StatusBadGateway)
	}

	var keySet jwks
	err = json.NewDecoder(jwksResp.Body).Decode(&keySet)
	if err != nil {
		return err
	}

	var matchedKey *jwk
	for i := range keySet.Keys {
		key := &keySet.Keys[i]
		if key.Kid == header.Kid {
			matchedKey = key
			break
		}
	}

	if matchedKey == nil {
		return c.NoContent(http.StatusUnauthorized)
	}

	if matchedKey.Kty != "RSA" {
		return c.NoContent(http.StatusUnauthorized)
	}

	if matchedKey.Alg != "" && matchedKey.Alg != "RS256" {
		return c.NoContent(http.StatusUnauthorized)
	}

	if matchedKey.Use != "" && matchedKey.Use != "sig" {
		return c.NoContent(http.StatusUnauthorized)
	}

	nBytes, err := base64.RawURLEncoding.DecodeString(matchedKey.N)
	if err != nil {
		return err
	}

	eBytes, err := base64.RawURLEncoding.DecodeString(matchedKey.E)
	if err != nil {
		return err
	}

	modulus := new(big.Int).SetBytes(nBytes)

	exponent := 0
	for _, b := range eBytes {
		exponent = exponent<<8 + int(b)
	}

	if modulus.Sign() <= 0 || exponent == 0 {
		return c.NoContent(http.StatusUnauthorized)
	}

	publicKey := &rsa.PublicKey{
		N: modulus,
		E: exponent,
	}

	signingInput := parts[0] + "." + parts[1]
	hashed := sha256.Sum256([]byte(signingInput))

	err = rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, hashed[:], signatureBytes)
	if err != nil {
		return c.NoContent(http.StatusUnauthorized)
	}

	if claims.Iss != keycloakAuthBase {
		return c.NoContent(http.StatusUnauthorized)
	}

	if claims.Aud != clientID {
		return c.NoContent(http.StatusUnauthorized)
	}

	if claims.Exp <= time.Now().Unix() {
		return c.NoContent(http.StatusUnauthorized)
	}

	sd := sessionData{
		Sub:               claims.Sub,
		PreferredUsername: claims.PreferredUsername,
		IDToken:           tokens.IDToken,
		CSRFToken:         uuid.NewString(),
	}

	id := uuid.NewString()

	sessions[id] = sd

	cookie := &http.Cookie{
		Name:     "session_id",
		Value:    id,
		HttpOnly: true,
		Path:     "/",
		SameSite: http.SameSiteLaxMode,
	}

	c.SetCookie(cookie)

	return c.Redirect(http.StatusFound, frontendTopURL)
}

func handleCSRF(c echo.Context) error {

	sd := c.Get(ctxKeySession).(sessionData)

	return c.JSON(http.StatusOK, map[string]any{
		"csrf_token": sd.CSRFToken,
	})

}

func handleMe(c echo.Context) error {

	sd := c.Get(ctxKeySession).(sessionData)

	return c.JSON(http.StatusOK, map[string]any{
		"name": sd.PreferredUsername,
	})

}

func RequireSession(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		sessionCookie, err := c.Cookie("session_id")

		if err != nil {
			return c.NoContent(http.StatusUnauthorized)
		}

		sd, ok := sessions[sessionCookie.Value]

		if !ok {
			return c.NoContent(http.StatusUnauthorized)
		}

		c.Set(ctxKeySession, sd)
		c.Set(ctxKeySessionId, sessionCookie.Value)

		return next(c)

	}
}

func RequireCSRF(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {

		sd := c.Get(ctxKeySession).(sessionData)
		gotCSRF := c.Request().Header.Get("X-CSRF-Token")
		wantCSRF := sd.CSRFToken

		if gotCSRF == "" || subtle.ConstantTimeCompare([]byte(gotCSRF), []byte(wantCSRF)) != 1 {
			return c.NoContent(http.StatusForbidden)
		}

		return next(c)
	}
}
