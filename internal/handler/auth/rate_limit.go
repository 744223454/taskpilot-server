package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	logicerrors "github.com/744223454/taskpilot-server/internal/logic"
	"github.com/744223454/taskpilot-server/internal/svc"
)

func enforceRegisterRateLimit(ctx context.Context, svcCtx *svc.ServiceContext, clientIP string) (time.Duration, error) {
	return enforceAuthRateLimit(
		ctx,
		svcCtx,
		"register:"+rateLimitDigest(strings.TrimSpace(clientIP)),
		svcCtx.Config.Auth.RegisterRateLimit,
		time.Duration(svcCtx.Config.Auth.RegisterRateWindow)*time.Second,
	)
}

func enforceLoginRateLimit(ctx context.Context, svcCtx *svc.ServiceContext, clientIP, email string) (time.Duration, error) {
	identity := strings.TrimSpace(clientIP) + "\x00" + strings.ToLower(strings.TrimSpace(email))
	return enforceAuthRateLimit(
		ctx,
		svcCtx,
		"login:"+rateLimitDigest(identity),
		svcCtx.Config.Auth.LoginRateLimit,
		time.Duration(svcCtx.Config.Auth.LoginRateWindow)*time.Second,
	)
}

func enforceAuthRateLimit(ctx context.Context, svcCtx *svc.ServiceContext, key string, limit int64, window time.Duration) (time.Duration, error) {
	if svcCtx.AuthRateLimiter == nil {
		return 0, logicerrors.ErrCacheUnavailable
	}
	allowed, retryAfter, err := svcCtx.AuthRateLimiter.Allow(ctx, key, limit, window)
	if err != nil {
		return 0, fmt.Errorf("%w: enforce auth rate limit: %v", logicerrors.ErrCacheUnavailable, err)
	}
	if !allowed {
		return retryAfter, logicerrors.ErrRateLimited
	}
	return 0, nil
}

func setRetryAfterHeader(headerSetter interface{ Header(string, string) }, retryAfter time.Duration) {
	if retryAfter <= 0 {
		return
	}
	seconds := int64((retryAfter + time.Second - 1) / time.Second)
	headerSetter.Header("Retry-After", strconv.FormatInt(seconds, 10))
}

func rateLimitDigest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
