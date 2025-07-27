package client

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"time"
)

// JWTHeader represents the JWT header
type JWTHeader struct {
	Alg   string `json:"alg"`
	Typ   string `json:"typ"`
	Kid   string `json:"kid"`
	Nonce string `json:"nonce"`
}

// JWTClaims represents the JWT claims for Coinbase API
type JWTClaims struct {
	Sub string `json:"sub"`
	Iss string `json:"iss"`
	Exp int64  `json:"exp"`
	Iat int64  `json:"iat"`
	Uri string `json:"uri"`
}

// generateNonce creates a random integer nonce for JWT
func generateNonce() (string, error) {
	randomBytes := make([]byte, 8)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}
	randomInt := new(big.Int).SetBytes(randomBytes)
	return randomInt.String(), nil
}

// createJWT creates a JWT token signed with ECDSA (ES256)
func (c *CoinbaseClient) createJWT(method, endpoint string) (string, error) {
	nonce, err := generateNonce()
	if err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	header := JWTHeader{
		Alg:   "ES256",
		Typ:   "JWT",
		Kid:   c.apiKey,
		Nonce: nonce,
	}

	now := time.Now()
	uri := fmt.Sprintf("%s api.coinbase.com%s", method, endpoint)
	claims := JWTClaims{
		Sub: c.apiKey,
		Iss: "cdp",
		Exp: now.Add(120 * time.Second).Unix(),
		Iat: now.Unix(),
		Uri: uri,
	}

	// Encode header and claims
	headerBytes, err := json.Marshal(header)
	if err != nil {
		return "", fmt.Errorf("failed to marshal header: %w", err)
	}

	claimsBytes, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("failed to marshal claims: %w", err)
	}

	headerB64 := base64.RawURLEncoding.EncodeToString(headerBytes)
	claimsB64 := base64.RawURLEncoding.EncodeToString(claimsBytes)

	// Create payload to sign
	payload := headerB64 + "." + claimsB64

	// Hash and sign
	hasher := sha256.New()
	hasher.Write([]byte(payload))
	hash := hasher.Sum(nil)

	r, s, err := ecdsa.Sign(rand.Reader, c.privateKey, hash)
	if err != nil {
		return "", fmt.Errorf("failed to sign with ECDSA: %w", err)
	}

	// Convert r and s to fixed-length byte arrays (32 bytes each for P-256)
	rBytes := r.Bytes()
	sBytes := s.Bytes()

	rPadded := make([]byte, 32)
	sPadded := make([]byte, 32)
	copy(rPadded[32-len(rBytes):], rBytes)
	copy(sPadded[32-len(sBytes):], sBytes)

	signature := append(rPadded, sPadded...)
	signatureB64 := base64.RawURLEncoding.EncodeToString(signature)

	jwt := payload + "." + signatureB64

	// Debug output (only in DEBUG log level)
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "DEBUG" {
		headerPretty, _ := json.MarshalIndent(header, "", "  ")
		claimsPretty, _ := json.MarshalIndent(claims, "", "  ")
		c.logger.Printf("JWT Header: %s", string(headerPretty))
		c.logger.Printf("JWT Claims: %s", string(claimsPretty))
	}

	return jwt, nil
}
