package task

import (
	"errors"
	"testing"

	logicerrors "github.com/744223454/taskpilot-server/internal/logic"
	"github.com/744223454/taskpilot-server/internal/types"
)

func TestValidateReorderRequest(t *testing.T) {
	if err := validateReorderRequest(1, &types.ReorderTasksRequest{ProjectID: 2, TaskIDs: []int64{3, 4}}); err != nil {
		t.Fatalf("validateReorderRequest() error = %v", err)
	}
	for _, req := range []*types.ReorderTasksRequest{
		nil,
		{ProjectID: 2, TaskIDs: nil},
		{ProjectID: 2, TaskIDs: []int64{3, 3}},
		{ProjectID: 2, TaskIDs: []int64{0}},
	} {
		if err := validateReorderRequest(1, req); !errors.Is(err, logicerrors.ErrInvalidInput) {
			t.Fatalf("validateReorderRequest(%#v) error = %v, want invalid input", req, err)
		}
	}
}

func TestNormalizeTaskRequests(t *testing.T) {
	description := "  Description  "
	created, err := normalizeCreateRequest(1, 2, &types.CreateTaskRequest{
		Title:       "  Task  ",
		Description: &description,
		Priority:    "high",
	})
	if err != nil || created.Title != "Task" || created.Description == nil || *created.Description != "Description" {
		t.Fatalf("normalizeCreateRequest() = %#v, %v", created, err)
	}
	if _, err := normalizeUpdateRequest(1, 3, &types.UpdateTaskRequest{Version: 1, Title: "Task", Priority: "urgent"}); !errors.Is(err, logicerrors.ErrInvalidInput) {
		t.Fatalf("normalizeUpdateRequest() error = %v, want invalid input", err)
	}
}
