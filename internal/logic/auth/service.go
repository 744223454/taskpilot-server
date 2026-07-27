package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	logicerrors "github.com/744223454/taskpilot-server/internal/logic"
	"github.com/744223454/taskpilot-server/internal/svc"
	"github.com/744223454/taskpilot-server/internal/types"
	"github.com/744223454/taskpilot-server/model/usermodel"
	jwtauth "github.com/744223454/taskpilot-server/pkg/auth"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var ErrEmailRegistered = errors.New("email already registered")
var ErrInvalidCredentials = errors.New("invalid email or password")
var ErrInvalidAccessToken = errors.New("invalid access token")
var ErrInvalidRefreshToken = errors.New("invalid refresh token")
var ErrRefreshTokenReused = errors.New("refresh token reused")

type AuthSession struct {
	Response         *types.AuthResponse
	RefreshToken     string
	RefreshExpiresAt time.Time
}

type Service struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewService(ctx context.Context, svcCtx *svc.ServiceContext) *Service {
	return &Service{
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (s *Service) Register(req *types.RegisterRequest) (*AuthSession, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}
	if err := s.requireJWT(); err != nil {
		return nil, err
	}
	if err := s.requireRefreshSessions(); err != nil {
		return nil, err
	}

	email := normalizeEmail(req.Email)
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash registration password: %w", err)
	}

	var authSession *AuthSession
	err = s.svcCtx.DB.Transaction(func(tx *gorm.DB) error {
		user := usermodel.User{
			Email:        email,
			PasswordHash: string(hashedPassword),
			Nickname:     req.Nickname,
		}
		if createErr := gorm.G[usermodel.User](tx).Create(s.ctx, &user); createErr != nil {
			return createErr
		}

		issuedSession, issueErr := s.issueSession(user)
		if issueErr != nil {
			return issueErr
		}
		authSession = issuedSession
		return nil
	})
	if err != nil {
		if authSession != nil {
			s.revokeIssuedSession(authSession.RefreshToken)
		}
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, ErrEmailRegistered
		}
		if errors.Is(err, logicerrors.ErrCacheUnavailable) {
			return nil, err
		}
		return nil, fmt.Errorf("register user: %w", err)
	}
	return authSession, nil
}

func (s *Service) Login(req *types.LoginRequest) (*AuthSession, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}
	if err := s.requireJWT(); err != nil {
		return nil, err
	}
	if err := s.requireRefreshSessions(); err != nil {
		return nil, err
	}

	email := normalizeEmail(req.Email)
	user, err := gorm.G[usermodel.User](s.svcCtx.DB).
		Where("LOWER(email) = ?", email).
		First(s.ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("find user for login: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	return s.issueSession(user)
}

func (s *Service) Refresh(rawRefreshToken string) (*AuthSession, error) {
	if err := s.requireJWT(); err != nil {
		return nil, err
	}
	if err := s.requireRefreshSessions(); err != nil {
		return nil, err
	}

	currentToken, err := jwtauth.ParseRefreshToken(rawRefreshToken)
	if err != nil {
		return nil, ErrInvalidRefreshToken
	}
	replacementToken, err := jwtauth.GenerateRefreshToken(currentToken.SessionID)
	if err != nil {
		return nil, fmt.Errorf("generate replacement refresh token: %w", err)
	}

	refreshSession, err := s.svcCtx.RefreshSessions.Rotate(
		s.ctx,
		currentToken.SessionID,
		currentToken.Hash,
		replacementToken.Hash,
		time.Now().UTC(),
	)
	if err != nil {
		return nil, mapRefreshSessionError("rotate refresh session", err)
	}

	accessToken, err := s.svcCtx.JWT.GenerateToken(jwtauth.Claims{
		UserID:   refreshSession.UserID,
		Email:    refreshSession.Email,
		Nickname: refreshSession.Nickname,
	})
	if err != nil {
		return nil, fmt.Errorf("issue refreshed access token: %w", err)
	}
	return &AuthSession{
		Response: &types.AuthResponse{
			User: types.UserProfile{
				ID:        refreshSession.UserID,
				Email:     refreshSession.Email,
				Nickname:  refreshSession.Nickname,
				AvatarURL: refreshSession.AvatarURL,
			},
			AccessToken:  accessToken,
			ExpiresInSec: s.svcCtx.Config.Auth.AccessExpire,
		},
		RefreshToken:     replacementToken.Raw,
		RefreshExpiresAt: refreshSession.ExpiresAt,
	}, nil
}

func (s *Service) Logout(rawRefreshToken string) error {
	if err := s.requireRefreshSessions(); err != nil {
		return err
	}

	refreshToken, err := jwtauth.ParseRefreshToken(rawRefreshToken)
	if err != nil {
		return ErrInvalidRefreshToken
	}
	if err := s.svcCtx.RefreshSessions.Revoke(s.ctx, refreshToken.SessionID, refreshToken.Hash); err != nil {
		return mapRefreshSessionError("revoke refresh session", err)
	}
	return nil
}

func (s *Service) CurrentUserByID(userID int64) (*types.UserProfile, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}

	user, err := gorm.G[usermodel.User](s.svcCtx.DB).
		Where("id = ?", userID).
		First(s.ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrInvalidAccessToken
	}
	if err != nil {
		return nil, fmt.Errorf("find current user: %w", err)
	}

	return &types.UserProfile{
		ID:        user.ID,
		Email:     user.Email,
		Nickname:  user.Nickname,
		AvatarURL: user.AvatarURL,
	}, nil
}

func issueToken(manager *jwtauth.Manager, user usermodel.User) (string, error) {
	return manager.GenerateToken(jwtauth.Claims{
		UserID:   user.ID,
		Email:    user.Email,
		Nickname: user.Nickname,
	})
}

func (s *Service) issueSession(user usermodel.User) (*AuthSession, error) {
	accessToken, err := issueToken(s.svcCtx.JWT, user)
	if err != nil {
		return nil, fmt.Errorf("issue access token: %w", err)
	}
	refreshToken, err := jwtauth.GenerateRefreshToken("")
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}
	expiresAt := time.Now().UTC().Add(time.Duration(s.svcCtx.Config.Auth.RefreshExpire) * time.Second)
	if err := s.svcCtx.RefreshSessions.Create(s.ctx, jwtauth.RefreshSession{
		ID:        refreshToken.SessionID,
		UserID:    user.ID,
		Email:     user.Email,
		Nickname:  user.Nickname,
		AvatarURL: user.AvatarURL,
		ExpiresAt: expiresAt,
	}, refreshToken.Hash); err != nil {
		return nil, mapRefreshSessionError("create refresh session", err)
	}
	return &AuthSession{
		Response:         newAuthResponse(user, accessToken, s.svcCtx.Config.Auth.AccessExpire),
		RefreshToken:     refreshToken.Raw,
		RefreshExpiresAt: expiresAt,
	}, nil
}

func (s *Service) revokeIssuedSession(rawRefreshToken string) {
	refreshToken, err := jwtauth.ParseRefreshToken(rawRefreshToken)
	if err != nil {
		return
	}
	if err := s.svcCtx.RefreshSessions.Revoke(s.ctx, refreshToken.SessionID, refreshToken.Hash); err != nil && s.svcCtx.Logger != nil {
		s.svcCtx.Logger.ErrorContext(s.ctx, "failed to clean up refresh session after registration rollback", "error", err)
	}
}

func mapRefreshSessionError(operation string, err error) error {
	switch {
	case errors.Is(err, jwtauth.ErrRefreshSessionNotFound):
		return ErrInvalidRefreshToken
	case errors.Is(err, jwtauth.ErrRefreshTokenReused):
		return ErrRefreshTokenReused
	default:
		return fmt.Errorf("%w: %s: %v", logicerrors.ErrCacheUnavailable, operation, err)
	}
}

func newAuthResponse(user usermodel.User, token string, expiresInSec int64) *types.AuthResponse {
	return &types.AuthResponse{
		User: types.UserProfile{
			ID:        user.ID,
			Email:     user.Email,
			Nickname:  user.Nickname,
			AvatarURL: user.AvatarURL,
		},
		AccessToken:  token,
		ExpiresInSec: expiresInSec,
	}
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func (s *Service) requireDB() error {
	if s.svcCtx.DB == nil {
		return logicerrors.ErrDatabaseUnavailable
	}
	return nil
}

func (s *Service) requireJWT() error {
	return s.svcCtx.JWT.Validate()
}

func (s *Service) requireRefreshSessions() error {
	if s.svcCtx.RefreshSessions == nil || s.svcCtx.Config.Auth.RefreshExpire <= 0 {
		return logicerrors.ErrCacheUnavailable
	}
	return nil
}
