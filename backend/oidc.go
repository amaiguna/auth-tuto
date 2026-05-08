package main

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func exchangeCodeForTokens(code string) (tokenResponse, error) {
	tokenURL, _ := url.Parse(keycloakTokenBase + "/protocol/openid-connect/token")

	resp, err := http.PostForm(tokenURL.String(), url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
	})
	if err != nil {
		return tokenResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return tokenResponse{}, errOIDCUpstream
	}

	var tokens tokenResponse
	err = json.NewDecoder(resp.Body).Decode(&tokens)
	if err != nil {
		return tokenResponse{}, err
	}

	return tokens, nil
}

func verifyIDToken(idToken string) (idTokenClaims, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return idTokenClaims{}, errInvalidIDToken
	}

	header, err := parseJWTHeader(parts[0])
	if err != nil {
		return idTokenClaims{}, err
	}

	claims, err := parseIDTokenClaims(parts[1])
	if err != nil {
		return idTokenClaims{}, err
	}

	signatureBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return idTokenClaims{}, err
	}

	keySet, err := fetchJWKS()
	if err != nil {
		return idTokenClaims{}, err
	}

	matchedKey, err := findJWKByKid(keySet, header.Kid)
	if err != nil {
		return idTokenClaims{}, err
	}

	publicKey, err := rsaPublicKeyFromJWK(*matchedKey)
	if err != nil {
		return idTokenClaims{}, err
	}

	if err := verifyJWTSignature(parts, publicKey, signatureBytes); err != nil {
		return idTokenClaims{}, err
	}

	if err := validateIDTokenClaims(claims); err != nil {
		return idTokenClaims{}, err
	}

	return claims, nil
}

func parseJWTHeader(encodedHeader string) (jwtHeader, error) {
	headerBytes, err := base64.RawURLEncoding.DecodeString(encodedHeader)
	if err != nil {
		return jwtHeader{}, err
	}

	var header jwtHeader
	err = json.Unmarshal(headerBytes, &header)
	if err != nil {
		return jwtHeader{}, err
	}

	if header.Alg != "RS256" {
		return jwtHeader{}, errInvalidIDToken
	}

	if header.Kid == "" {
		return jwtHeader{}, errInvalidIDToken
	}

	return header, nil
}

func parseIDTokenClaims(encodedPayload string) (idTokenClaims, error) {
	payloadBytes, err := base64.RawURLEncoding.DecodeString(encodedPayload)
	if err != nil {
		return idTokenClaims{}, err
	}

	var claims idTokenClaims
	err = json.Unmarshal(payloadBytes, &claims)
	if err != nil {
		return idTokenClaims{}, err
	}

	return claims, nil
}

func fetchJWKS() (jwks, error) {
	publicKeyURL, _ := url.Parse(keycloakTokenBase + "/protocol/openid-connect/certs")

	jwksResp, err := http.Get(publicKeyURL.String())
	if err != nil {
		return jwks{}, fmt.Errorf("%w: %v", errOIDCUpstream, err)
	}
	defer jwksResp.Body.Close()

	if jwksResp.StatusCode != http.StatusOK {
		return jwks{}, errOIDCUpstream
	}

	var keySet jwks
	err = json.NewDecoder(jwksResp.Body).Decode(&keySet)
	if err != nil {
		return jwks{}, err
	}

	return keySet, nil
}

func findJWKByKid(keySet jwks, kid string) (*jwk, error) {
	for i := range keySet.Keys {
		key := &keySet.Keys[i]
		if key.Kid == kid {
			return key, nil
		}
	}

	return nil, errInvalidIDToken
}

func rsaPublicKeyFromJWK(key jwk) (*rsa.PublicKey, error) {
	if key.Kty != "RSA" {
		return nil, errInvalidIDToken
	}

	if key.Alg != "" && key.Alg != "RS256" {
		return nil, errInvalidIDToken
	}

	if key.Use != "" && key.Use != "sig" {
		return nil, errInvalidIDToken
	}

	nBytes, err := base64.RawURLEncoding.DecodeString(key.N)
	if err != nil {
		return nil, err
	}

	eBytes, err := base64.RawURLEncoding.DecodeString(key.E)
	if err != nil {
		return nil, err
	}

	modulus := new(big.Int).SetBytes(nBytes)

	exponent := 0
	for _, b := range eBytes {
		exponent = exponent<<8 + int(b)
	}

	if modulus.Sign() <= 0 || exponent == 0 {
		return nil, errInvalidIDToken
	}

	return &rsa.PublicKey{
		N: modulus,
		E: exponent,
	}, nil
}

func verifyJWTSignature(parts []string, publicKey *rsa.PublicKey, signatureBytes []byte) error {
	signingInput := parts[0] + "." + parts[1]
	hashed := sha256.Sum256([]byte(signingInput))

	err := rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, hashed[:], signatureBytes)
	if err != nil {
		return errInvalidIDToken
	}

	return nil
}

func validateIDTokenClaims(claims idTokenClaims) error {
	if claims.Iss != keycloakAuthBase {
		return errInvalidIDToken
	}

	if claims.Aud != clientID {
		return errInvalidIDToken
	}

	if claims.Exp <= time.Now().Unix() {
		return errInvalidIDToken
	}

	return nil
}
