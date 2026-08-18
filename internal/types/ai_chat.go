package types

type AIChatMessage struct {
	Role    string `json:"role" binding:"required,oneof=user assistant"`
	Content string `json:"content" binding:"required"`
}

type AIChatRequest struct {
	ParseResultID int64           `json:"parse_result_id" binding:"required,gt=0"`
	Messages      []AIChatMessage `json:"messages" binding:"required,min=1,max=16,dive"`
}
