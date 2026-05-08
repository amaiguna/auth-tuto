package main

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

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

func validateOAuthState(c echo.Context) error {
	stateFromQuery := c.QueryParam("state")
	stateCookie, err := c.Cookie("oauth_state")
	if err != nil || stateCookie.Value != stateFromQuery {
		return errInvalidOAuthState
	}

	c.SetCookie(&http.Cookie{
		Name:   "oauth_state",
		MaxAge: -1,
		Path:   "/",
	})

	return nil
}

func newSessionData(claims idTokenClaims, idToken string) sessionData {
	return sessionData{
		Sub:               claims.Sub,
		PreferredUsername: claims.PreferredUsername,
		IDToken:           idToken,
		CSRFToken:         uuid.NewString(),
	}
}

func setSessionCookie(c echo.Context, sessionID string) {
	cookie := &http.Cookie{
		Name:     "session_id",
		Value:    sessionID,
		HttpOnly: true,
		Path:     "/",
		SameSite: http.SameSiteLaxMode,
	}

	c.SetCookie(cookie)
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
