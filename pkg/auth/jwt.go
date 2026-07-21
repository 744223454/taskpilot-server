// Package auth provides JWT generation and validation helpers.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var ErrInvalidJWTConfig = errors.New("jwt manager is misconfigured")
var ErrInvalidToken = errors.New("invalid token")
var ErrExpiredToken = errors.New("token expired")

type Claims struct {
	UserID   int64  `json:"user_id"`
	Email    string `json:"email"`
	Nickname string `json:"nickname"`
}

type Manager struct {
	secret        string
	expireSeconds int64
	now           func() time.Time
}

func NewManager(secret string, expireSeconds int64) *Manager {
	return &Manager{
		secret:        secret,
		expireSeconds: expireSeconds,
		now:           time.Now,
	}
}

func (m *Manager) Validate() error {
	if m == nil || m.secret == "" || m.expireSeconds <= 0 {
		return ErrInvalidJWTConfig
	}
	return nil
}

func (m *Manager) GenerateToken(claims Claims) (string, error) {
	if err := m.Validate(); err != nil {
		return "", err
	}

	headerJSON, err := json.Marshal(jwtHeader{
		Alg: "HS256",
		Typ: "JWT",
	})
	if err != nil {
		return "", err
	}

	now := m.now().Unix()
	payloadJSON, err := json.Marshal(jwtPayload{
		Claims: claims,
		Exp:    now + m.expireSeconds,
		Iat:    now,
	})
	if err != nil {
		return "", err
	}

	header := encodeSegment(headerJSON)
	payload := encodeSegment(payloadJSON)
	signingInput := header + "." + payload

	signature := signHS256(signingInput, m.secret)
	return signingInput + "." + encodeSegment(signature), nil
}

func (m *Manager) ParseToken(token string) (*Claims, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	token = strings.TrimSpace(token)
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, ErrInvalidToken
	}

	signingInput := parts[0] + "." + parts[1]
	expectedSignature := signHS256(signingInput, m.secret)
	actualSignature, err := decodeSegment(parts[2])
	if err != nil {
		return nil, ErrInvalidToken
	}
	if !hmac.Equal(actualSignature, expectedSignature) {
		return nil, ErrInvalidToken
	}

	headerBytes, err := decodeSegment(parts[0])
	if err != nil {
		return nil, ErrInvalidToken
	}

	var header jwtHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, ErrInvalidToken
	}
	if header.Alg != "HS256" || header.Typ != "JWT" {
		return nil, ErrInvalidToken
	}

	payloadBytes, err := decodeSegment(parts[1])
	if err != nil {
		return nil, ErrInvalidToken
	}

	var payload jwtPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return nil, ErrInvalidToken
	}
	if payload.Exp <= 0 {
		return nil, ErrInvalidToken
	}
	if m.now().Unix() >= payload.Exp {
		return nil, ErrExpiredToken
	}

	return &payload.Claims, nil
}

type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

type jwtPayload struct {
	Claims
	Exp int64 `json:"exp"`
	Iat int64 `json:"iat"`
}

func encodeSegment(raw []byte) string {
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeSegment(segment string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(segment)
}

func signHS256(signingInput string, secret string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(signingInput))
	return mac.Sum(nil)
}
