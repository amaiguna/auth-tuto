package main

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
