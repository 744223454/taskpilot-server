package cache

import (
	"context"
	"errors"
	"testing"
	"time"

	jwtauth "github.com/744223454/taskpilot-server/pkg/auth"
	"github.com/alicebob/miniredis/v2"
)

func TestRefreshSessionStoreRotatesAndRejectsReplay(t *testing.T) {
	redisServer, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer redisServer.Close()

	store := NewRefreshSessionStore(NewRedis(redisServer.Addr(), ""))
	initialToken, err := jwtauth.GenerateRefreshToken("")
	if err != nil {
		t.Fatalf("GenerateRefreshToken() error = %v", err)
	}
	expiresAt := time.Now().UTC().Add(time.Hour)
	session := jwtauth.RefreshSession{
		ID:        initialToken.SessionID,
		UserID:    42,
		Email:     "user@example.com",
		Nickname:  "tester",
		ExpiresAt: expiresAt,
	}
	if err := store.Create(context.Background(), session, initialToken.Hash); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	replacementToken, err := jwtauth.GenerateRefreshToken(initialToken.SessionID)
	if err != nil {
		t.Fatalf("GenerateRefreshToken() replacement error = %v", err)
	}
	rotated, err := store.Rotate(context.Background(), initialToken.SessionID, initialToken.Hash, replacementToken.Hash, time.Now().UTC())
	if err != nil {
		t.Fatalf("Rotate() error = %v", err)
	}
	if rotated.UserID != session.UserID || rotated.Email != session.Email || rotated.Nickname != session.Nickname {
		t.Fatalf("rotated session = %#v, want user profile from original session", rotated)
	}

	if _, err := store.Rotate(context.Background(), initialToken.SessionID, initialToken.Hash, initialToken.Hash, time.Now().UTC()); !errors.Is(err, jwtauth.ErrRefreshTokenReused) {
		t.Fatalf("replayed Rotate() error = %v, want %v", err, jwtauth.ErrRefreshTokenReused)
	}
	if _, err := store.Rotate(context.Background(), initialToken.SessionID, replacementToken.Hash, initialToken.Hash, time.Now().UTC()); !errors.Is(err, jwtauth.ErrRefreshSessionNotFound) {
		t.Fatalf("Rotate() after replay error = %v, want %v", err, jwtauth.ErrRefreshSessionNotFound)
	}
}

func TestRefreshSessionStoreRevoke(t *testing.T) {
	redisServer, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer redisServer.Close()

	store := NewRefreshSessionStore(NewRedis(redisServer.Addr(), ""))
	token, err := jwtauth.GenerateRefreshToken("")
	if err != nil {
		t.Fatalf("GenerateRefreshToken() error = %v", err)
	}
	if err := store.Create(context.Background(), jwtauth.RefreshSession{
		ID:        token.SessionID,
		UserID:    1,
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}, token.Hash); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := store.Revoke(context.Background(), token.SessionID, token.Hash); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	if err := store.Revoke(context.Background(), token.SessionID, token.Hash); !errors.Is(err, jwtauth.ErrRefreshSessionNotFound) {
		t.Fatalf("second Revoke() error = %v, want %v", err, jwtauth.ErrRefreshSessionNotFound)
	}
}

func TestRefreshSessionStoreRotatesProfileAndValidatesUser(t *testing.T) {
	redisServer, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer redisServer.Close()

	store := NewRefreshSessionStore(NewRedis(redisServer.Addr(), ""))
	initialToken, err := jwtauth.GenerateRefreshToken("")
	if err != nil {
		t.Fatalf("GenerateRefreshToken() error = %v", err)
	}
	if err := store.Create(context.Background(), jwtauth.RefreshSession{
		ID:        initialToken.SessionID,
		UserID:    42,
		Email:     "user@example.com",
		Nickname:  "before",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}, initialToken.Hash); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	replacementToken, err := jwtauth.GenerateRefreshToken(initialToken.SessionID)
	if err != nil {
		t.Fatalf("GenerateRefreshToken() replacement error = %v", err)
	}
	if _, err := store.RotateProfile(context.Background(), initialToken.SessionID, initialToken.Hash, replacementToken.Hash, 7, "attacker", nil, time.Now().UTC()); !errors.Is(err, jwtauth.ErrRefreshSessionNotFound) {
		t.Fatalf("wrong-user RotateProfile() error = %v", err)
	}
	avatarURL := "https://example.com/avatar.png"
	rotated, err := store.RotateProfile(context.Background(), initialToken.SessionID, initialToken.Hash, replacementToken.Hash, 42, "after", &avatarURL, time.Now().UTC())
	if err != nil {
		t.Fatalf("RotateProfile() error = %v", err)
	}
	if rotated.Nickname != "after" || rotated.AvatarURL == nil || *rotated.AvatarURL != avatarURL {
		t.Fatalf("rotated profile = %#v", rotated)
	}
}
