package project

import (
	"encoding/json"
	"errors"
	"testing"

	logicerrors "github.com/744223454/taskpilot-server/internal/logic"
	"github.com/744223454/taskpilot-server/internal/types"
)

func TestDecodeGeneratedTasksNormalizesDefaults(t *testing.T) {
	description := "  Prepare the final report  "
	raw, err := json.Marshal([]types.GeneratedTask{{
		Title:       "  Write report  ",
		Description: &description,
	}})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	tasks, err := decodeGeneratedTasks(raw)
	if err != nil {
		t.Fatalf("decodeGeneratedTasks() error = %v", err)
	}
	if len(tasks) != 1 || tasks[0].Title != "Write report" || tasks[0].Priority != "medium" || tasks[0].Description == nil || *tasks[0].Description != "Prepare the final report" {
		t.Fatalf("tasks = %#v", tasks)
	}
}

func TestDecodeGeneratedTasksAllowsEmptyArray(t *testing.T) {
	tasks, err := decodeGeneratedTasks(json.RawMessage(`[]`))
	if err != nil {
		t.Fatalf("decodeGeneratedTasks() error = %v", err)
	}
	if tasks == nil || len(tasks) != 0 {
		t.Fatalf("tasks = %#v, want non-nil empty slice", tasks)
	}
}

func TestDecodeGeneratedTasksRejectsInvalidSnapshot(t *testing.T) {
	for _, raw := range []json.RawMessage{
		json.RawMessage(`null`),
		json.RawMessage(`{"title":"not-an-array"}`),
		json.RawMessage(`[{"title":"","priority":"medium"}]`),
		json.RawMessage(`[{"title":"Task","priority":"urgent"}]`),
		json.RawMessage(`[{"title":"Task","unknown":true}]`),
	} {
		if _, err := decodeGeneratedTasks(raw); !errors.Is(err, logicerrors.ErrInvalidState) {
			t.Fatalf("decodeGeneratedTasks(%s) error = %v, want invalid state", raw, err)
		}
	}
}

func TestNormalizeRequest(t *testing.T) {
	name, err := normalizeRequest(1, &types.CreateProjectRequest{ParseResultID: 2, Name: "  Launch plan  "})
	if err != nil || name != "Launch plan" {
		t.Fatalf("normalizeRequest() = %q, %v", name, err)
	}
	if _, err := normalizeRequest(1, &types.CreateProjectRequest{ParseResultID: 2, Name: "   "}); !errors.Is(err, logicerrors.ErrInvalidInput) {
		t.Fatalf("normalizeRequest() error = %v, want invalid input", err)
	}
}
