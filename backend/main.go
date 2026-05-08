package main

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

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
