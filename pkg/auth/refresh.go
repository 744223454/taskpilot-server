package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

const (
	AccessCookieName  = "access_token"
	RefreshCookieName = "refresh_token"
	CSRFCookieName    = "csrf_token"
	CSRFHeaderName    = "X-CSRF-Token"
)

var ErrInvalidRefreshToken = errors.New("invalid refresh token")
var ErrRefreshSessionNotFound = errors.New("refresh session not found")
var ErrRefreshTokenReused = errors.New("refresh token reused")

type RefreshSession struct {
	ID        string
	UserID    int64
	Email     string
	Nickname  string
	AvatarURL *string
	ExpiresAt time.Time
}

type RefreshSessionStore interface {
	Create(ctx context.Context, session RefreshSession, tokenHash string) error
	Rotate(ctx context.Context, sessionID, currentHash, replacementHash string, now time.Time) (RefreshSession, error)
	RotateProfile(ctx context.Context, sessionID, currentHash, replacementHash string, userID int64, nickname string, avatarURL *string, now time.Time) (RefreshSession, error)
	Revoke(ctx context.Context, sessionID, tokenHash string) error
}

type RefreshToken struct {
	Raw       string
	SessionID string
	Hash      string
}

func GenerateRefreshToken(sessionID string) (RefreshToken, error) {
	if sessionID == "" {
		generatedSessionID, err := randomSegment()
		if err != nil {
			return RefreshToken{}, err
		}
		sessionID = generatedSessionID
	}
	if !validSegment(sessionID) {
		return RefreshToken{}, ErrInvalidRefreshToken
	}

	secret, err := randomSegment()
	if err != nil {
		return RefreshToken{}, err
	}
	raw := sessionID + "." + secret
	return RefreshToken{Raw: raw, SessionID: sessionID, Hash: hashToken(raw)}, nil
}

func ParseRefreshToken(raw string) (RefreshToken, error) {
	raw = strings.TrimSpace(raw)
	parts := strings.Split(raw, ".")
	if len(parts) != 2 || !validSegment(parts[0]) || !validSegment(parts[1]) {
		return RefreshToken{}, ErrInvalidRefreshToken
	}
	return RefreshToken{Raw: raw, SessionID: parts[0], Hash: hashToken(raw)}, nil
}

func GenerateCSRFToken() (string, error) {
	return randomSegment()
}

func randomSegment() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func validSegment(value string) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(decoded) == 32
}

func hashToken(raw string) string {
	digest := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(digest[:])
}
