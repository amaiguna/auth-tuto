package main

import "errors"

var (
	errInvalidOAuthState = errors.New("invalid oauth state")
	errInvalidIDToken    = errors.New("invalid id token")
	errOIDCUpstream      = errors.New("oidc upstream error")
)
