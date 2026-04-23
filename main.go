package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

const (
	keycloakBase          = "http://localhost:8080/realms/auth-tuto"
	clientID              = "echo-app"
	clientSecret          = "supersecret"
	redirectURI           = "http://localhost:3000/callback"
	postLogoutRedirectURI = "http://localhost:3000/loggedout"
)

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	IDToken      string `json:"id_token"`
	RefreshToken string `json:"refresh_token"`
}

type sessionData struct {
	Sub               string `json:"sub"`
	PreferredUsername string `json:"preferred_username"`
	IDToken           string `json:"id_token"`
}

// HACK: スレッドセーフでないのでsync.Mapに書き換え(必要なら)
var sessions = map[string]sessionData{}

func main() {
	e := echo.New()
	e.Use(middleware.RequestLogger())

	e.GET("/login", handleLogin)
	e.POST("/logout", handleLogout)
	e.GET("/loggedout", handleLoggedout)
	e.GET("/callback", handleCallback)
	e.GET("/me", handleMe)

	e.Start(":3000")
}

func handleLogin(c echo.Context) error {
	state := uuid.NewString()

	cookie := &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		HttpOnly: true,
		Path:     "/",
	}
	c.SetCookie(cookie)

	authURL, _ := url.Parse(keycloakBase + "/protocol/openid-connect/auth")

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
	sessionCookie, err := c.Cookie("session_id")

	if err != nil {
		return err
	}

	sessionData, ok := sessions[sessionCookie.Value]

	if !ok {
		return c.NoContent(http.StatusUnauthorized)
	}

	logoutURL, _ := url.Parse(keycloakBase + "/protocol/openid-connect/logout")
	q := logoutURL.Query()
	q.Set("id_token_hint", sessionData.IDToken)
	q.Set("post_logout_redirect_uri", postLogoutRedirectURI)

	logoutURL.RawQuery = q.Encode()

	delete(sessions, sessionCookie.Value)

	cookie := &http.Cookie{
		Name:   "session_id",
		Value:  "",
		MaxAge: -1,
		Path:   "/",
	}

	c.SetCookie(cookie)

	return c.Redirect(http.StatusFound, logoutURL.String())

}

func handleLoggedout(c echo.Context) error {
	return c.String(http.StatusOK, "ログアウトしました")
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

	tokenURL, _ := url.Parse(keycloakBase + "/protocol/openid-connect/token")

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

	var tokens tokenResponse
	err = json.NewDecoder(resp.Body).Decode(&tokens)
	if err != nil {
		return err
	}

	parts := strings.Split(tokens.IDToken, ".")
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return err
	}

	var sessionData sessionData
	err = json.Unmarshal(payloadBytes, &sessionData)
	if err != nil {
		return err
	}

	sessionData.IDToken = tokens.IDToken

	id := uuid.NewString()

	sessions[id] = sessionData

	cookie := &http.Cookie{
		Name:     "session_id",
		Value:    id,
		HttpOnly: true,
		Path:     "/",
	}

	c.SetCookie(cookie)

	return c.Redirect(http.StatusFound, "/me")
}

func handleMe(c echo.Context) error {

	sessionCookie, err := c.Cookie("session_id")

	if err != nil {
		return err
	}

	sessionData, ok := sessions[sessionCookie.Value]

	if !ok {
		return c.NoContent(http.StatusUnauthorized)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"name": sessionData.PreferredUsername,
	})

}
