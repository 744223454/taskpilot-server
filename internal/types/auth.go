package types

type RegisterRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
	Nickname string `json:"nickname" binding:"required,min=1,max=64"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
}

type UserProfile struct {
	ID        int64   `json:"id"`
	Email     string  `json:"email"`
	Nickname  string  `json:"nickname"`
	AvatarURL *string `json:"avatar_url,omitempty"`
}

type AuthResponse struct {
	User         UserProfile `json:"user"`
	AccessToken  string      `json:"access_token"`
	ExpiresInSec int64       `json:"expires_in_sec"`
}
