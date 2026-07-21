package auth

import (
	"context"
	"errors"
	"strings"

	"github.com/744223454/taskpilot-server/internal/svc"
	"github.com/744223454/taskpilot-server/internal/types"
	"github.com/744223454/taskpilot-server/model/usermodel"
	jwtauth "github.com/744223454/taskpilot-server/pkg/auth"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var ErrEmailRegistered = errors.New("email already registered")
var ErrDatabaseNotConnected = errors.New("database not connected")
var ErrInvalidCredentials = errors.New("invalid email or password")
var ErrInvalidAccessToken = errors.New("invalid access token")

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

func (s *Service) Register(req *types.RegisterRequest) (*types.AuthResponse, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}
	if err := s.requireJWT(); err != nil {
		return nil, err
	}

	email := normalizeEmail(req.Email)
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := usermodel.User{
		Email:        email,
		PasswordHash: string(hashedPassword),
		Nickname:     req.Nickname,
	}
	if err := gorm.G[usermodel.User](s.svcCtx.DB).
		Create(s.ctx, &user); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, ErrEmailRegistered
		}
		return nil, err
	}

	token, err := issueToken(s.svcCtx.JWT, user)
	if err != nil {
		return nil, err
	}

	return newAuthResponse(user, token, s.svcCtx.Config.Auth.AccessExpire), nil
}

func (s *Service) Login(req *types.LoginRequest) (*types.AuthResponse, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}
	if err := s.requireJWT(); err != nil {
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
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	token, err := issueToken(s.svcCtx.JWT, user)
	if err != nil {
		return nil, err
	}

	return newAuthResponse(user, token, s.svcCtx.Config.Auth.AccessExpire), nil
}

func (s *Service) CurrentUser(accessToken string) (*types.UserProfile, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}

	claims, err := s.svcCtx.JWT.ParseToken(accessToken)
	if errors.Is(err, jwtauth.ErrInvalidToken) || errors.Is(err, jwtauth.ErrExpiredToken) {
		return nil, ErrInvalidAccessToken
	}
	if err != nil {
		return nil, err
	}

	user, err := gorm.G[usermodel.User](s.svcCtx.DB).
		Where("id = ?", claims.UserID).
		First(s.ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrInvalidAccessToken
	}
	if err != nil {
		return nil, err
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
		return ErrDatabaseNotConnected
	}
	return nil
}

func (s *Service) requireJWT() error {
	return s.svcCtx.JWT.Validate()
}
