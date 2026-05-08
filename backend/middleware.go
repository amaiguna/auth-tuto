package main

import (
	"crypto/subtle"
	"net/http"

	"github.com/labstack/echo/v4"
)

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
