package types

import "time"

type TaskListRequest struct {
	Status string `form:"status" binding:"omitempty,oneof=todo doing done"`
}

type CreateTaskRequest struct {
	Title       string     `json:"title" binding:"required,max=255"`
	Description *string    `json:"description" binding:"omitempty,max=2000"`
	Priority    string     `json:"priority" binding:"required,oneof=low medium high"`
	Deadline    *time.Time `json:"deadline"`
}

type UpdateTaskRequest struct {
	Version     int32      `json:"version" binding:"required,min=1"`
	Title       string     `json:"title" binding:"required,max=255"`
	Description *string    `json:"description" binding:"omitempty,max=2000"`
	Priority    string     `json:"priority" binding:"required,oneof=low medium high"`
	Deadline    *time.Time `json:"deadline"`
}

type UpdateTaskStatusRequest struct {
	Status string `json:"status" binding:"required,oneof=todo doing done"`
}

type ReorderTasksRequest struct {
	ProjectID int64   `json:"project_id" binding:"required,min=1"`
	TaskIDs   []int64 `json:"task_ids"`
}

type TaskListResponse struct {
	Items []TaskResponse `json:"items"`
}
