package logic_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	logicerrors "github.com/744223454/taskpilot-server/internal/logic"
	projectlogic "github.com/744223454/taskpilot-server/internal/logic/project"
	tasklogic "github.com/744223454/taskpilot-server/internal/logic/task"
	"github.com/744223454/taskpilot-server/internal/svc"
	"github.com/744223454/taskpilot-server/internal/types"
	"github.com/744223454/taskpilot-server/model/taskmodel"
	"gorm.io/gorm"
)

func TestIntegrationProjectLifecycleAndOptimisticLock(t *testing.T) {
	db := newIntegrationDB(t)
	ctx := context.Background()
	serviceContext := &svc.ServiceContext{DB: db}
	user := createIntegrationUser(t, db, "project-lifecycle@example.com")
	result := createIntegrationParseResult(t, db, user.ID, true, json.RawMessage(`[]`))
	service := projectlogic.NewService(ctx, serviceContext)

	created, _, err := service.Create(user.ID, &types.CreateProjectRequest{ParseResultID: result.ID, Name: "Lifecycle"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	description := "  Active project  "
	updated, err := service.Update(user.ID, created.Project.ID, &types.UpdateProjectRequest{
		Version:     created.Project.Version,
		Name:        "  Updated lifecycle  ",
		Description: &description,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Name != "Updated lifecycle" || updated.Description == nil || *updated.Description != "Active project" || updated.Version != created.Project.Version+1 {
		t.Fatalf("updated project = %#v", updated)
	}
	if _, err := service.Update(user.ID, created.Project.ID, &types.UpdateProjectRequest{Version: created.Project.Version, Name: "Stale"}); !errors.Is(err, logicerrors.ErrConflict) {
		t.Fatalf("stale Update() error = %v, want conflict", err)
	}
	if err := service.Delete(user.ID, created.Project.ID); !errors.Is(err, logicerrors.ErrConflict) {
		t.Fatalf("active Delete() error = %v, want conflict", err)
	}

	archived, err := service.Archive(user.ID, created.Project.ID)
	if err != nil || archived.Status != "archived" {
		t.Fatalf("Archive() = %#v, %v", archived, err)
	}
	archivedAgain, err := service.Archive(user.ID, created.Project.ID)
	if err != nil || archivedAgain.Version != archived.Version {
		t.Fatalf("idempotent Archive() = %#v, %v", archivedAgain, err)
	}
	if _, err := service.Update(user.ID, created.Project.ID, &types.UpdateProjectRequest{Version: archived.Version, Name: "Blocked"}); !errors.Is(err, logicerrors.ErrConflict) {
		t.Fatalf("archived Update() error = %v, want conflict", err)
	}

	active, err := service.Unarchive(user.ID, created.Project.ID)
	if err != nil || active.Status != "active" {
		t.Fatalf("Unarchive() = %#v, %v", active, err)
	}
	activeAgain, err := service.Unarchive(user.ID, created.Project.ID)
	if err != nil || activeAgain.Version != active.Version {
		t.Fatalf("idempotent Unarchive() = %#v, %v", activeAgain, err)
	}
	archived, err = service.Archive(user.ID, created.Project.ID)
	if err != nil {
		t.Fatalf("second Archive() error = %v", err)
	}
	if err := service.Delete(user.ID, created.Project.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := service.Get(user.ID, created.Project.ID); !errors.Is(err, logicerrors.ErrNotFound) {
		t.Fatalf("Get(deleted) error = %v, want not found", err)
	}
	history, err := service.HistoryGet(user.ID, created.Project.ID)
	if err != nil || history.Status != "deleted" {
		t.Fatalf("HistoryGet() = %#v, %v", history, err)
	}
	if err := service.Delete(user.ID, created.Project.ID); !errors.Is(err, logicerrors.ErrNotFound) {
		t.Fatalf("repeated Delete() error = %v, want not found", err)
	}
	if _, _, err := service.Create(user.ID, &types.CreateProjectRequest{ParseResultID: result.ID, Name: "Recreated"}); !errors.Is(err, logicerrors.ErrConflict) {
		t.Fatalf("Create() from deleted source error = %v, want conflict", err)
	}
}

func TestIntegrationTaskManagementAndArchivedReadOnly(t *testing.T) {
	db := newIntegrationDB(t)
	ctx := context.Background()
	serviceContext := &svc.ServiceContext{DB: db}
	user := createIntegrationUser(t, db, "task-management@example.com")
	result := createIntegrationParseResult(t, db, user.ID, true, json.RawMessage(`[{
		"title":"AI task","priority":"high"
	}]`))
	projectService := projectlogic.NewService(ctx, serviceContext)
	created, _, err := projectService.Create(user.ID, &types.CreateProjectRequest{ParseResultID: result.ID, Name: "Tasks"})
	if err != nil {
		t.Fatalf("Create project error = %v", err)
	}
	taskService := tasklogic.NewService(ctx, serviceContext)

	manual, err := taskService.Create(user.ID, created.Project.ID, &types.CreateTaskRequest{Title: "  Manual task  ", Priority: "medium"})
	if err != nil {
		t.Fatalf("Create task error = %v", err)
	}
	if manual.Status != "todo" || manual.SourceType != "manual" || manual.SourceParseResultID != nil || manual.SortOrder != 1 || manual.Version != 1 {
		t.Fatalf("manual task = %#v", manual)
	}
	description := "  Updated task description  "
	updated, err := taskService.Update(user.ID, manual.ID, &types.UpdateTaskRequest{
		Version:     manual.Version,
		Title:       "  Updated manual task  ",
		Description: &description,
		Priority:    "low",
	})
	if err != nil {
		t.Fatalf("Update task error = %v", err)
	}
	if updated.Title != "Updated manual task" || updated.Description == nil || *updated.Description != "Updated task description" || updated.Version != manual.Version+1 {
		t.Fatalf("updated task = %#v", updated)
	}
	if _, err := taskService.Update(user.ID, manual.ID, &types.UpdateTaskRequest{Version: manual.Version, Title: "Stale", Priority: "low"}); !errors.Is(err, logicerrors.ErrConflict) {
		t.Fatalf("stale task Update() error = %v, want conflict", err)
	}
	done, err := taskService.UpdateStatus(user.ID, manual.ID, &types.UpdateTaskStatusRequest{Status: "done"})
	if err != nil || done.Status != "done" {
		t.Fatalf("UpdateStatus(done) = %#v, %v", done, err)
	}
	doneAgain, err := taskService.UpdateStatus(user.ID, manual.ID, &types.UpdateTaskStatusRequest{Status: "done"})
	if err != nil || doneAgain.Version != done.Version {
		t.Fatalf("idempotent UpdateStatus() = %#v, %v", doneAgain, err)
	}
	if _, err := taskService.UpdateStatus(user.ID, manual.ID, &types.UpdateTaskStatusRequest{Status: "todo"}); err != nil {
		t.Fatalf("UpdateStatus(todo) error = %v", err)
	}

	if _, err := projectService.Archive(user.ID, created.Project.ID); err != nil {
		t.Fatalf("Archive project error = %v", err)
	}
	listed, err := taskService.List(user.ID, created.Project.ID, &types.TaskListRequest{})
	if err != nil || len(listed.Items) != 2 {
		t.Fatalf("List archived tasks = %#v, %v", listed, err)
	}
	if _, err := taskService.Create(user.ID, created.Project.ID, &types.CreateTaskRequest{Title: "Blocked", Priority: "medium"}); !errors.Is(err, logicerrors.ErrConflict) {
		t.Fatalf("archived Create() error = %v, want conflict", err)
	}
	if err := taskService.Delete(user.ID, manual.ID); !errors.Is(err, logicerrors.ErrConflict) {
		t.Fatalf("archived Delete() error = %v, want conflict", err)
	}
	if _, err := projectService.Unarchive(user.ID, created.Project.ID); err != nil {
		t.Fatalf("Unarchive project error = %v", err)
	}
	if err := taskService.Delete(user.ID, manual.ID); err != nil {
		t.Fatalf("Delete task error = %v", err)
	}
	if err := taskService.Delete(user.ID, manual.ID); !errors.Is(err, logicerrors.ErrNotFound) {
		t.Fatalf("repeated task Delete() error = %v, want not found", err)
	}
}

func TestIntegrationTaskReorderValidatesCompleteProjectSet(t *testing.T) {
	db := newIntegrationDB(t)
	ctx := context.Background()
	serviceContext := &svc.ServiceContext{DB: db}
	user := createIntegrationUser(t, db, "task-reorder@example.com")
	result := createIntegrationParseResult(t, db, user.ID, true, json.RawMessage(`[
		{"title":"First","priority":"medium"},
		{"title":"Second","priority":"medium"}
	]`))
	project, _, err := projectlogic.NewService(ctx, serviceContext).Create(user.ID, &types.CreateProjectRequest{ParseResultID: result.ID, Name: "Reorder"})
	if err != nil {
		t.Fatalf("Create project error = %v", err)
	}
	taskService := tasklogic.NewService(ctx, serviceContext)
	third, err := taskService.Create(user.ID, project.Project.ID, &types.CreateTaskRequest{Title: "Third", Priority: "medium"})
	if err != nil {
		t.Fatalf("Create third task error = %v", err)
	}
	firstID := project.Tasks[0].ID
	secondID := project.Tasks[1].ID
	reordered, err := taskService.Reorder(user.ID, &types.ReorderTasksRequest{ProjectID: project.Project.ID, TaskIDs: []int64{third.ID, firstID, secondID}})
	if err != nil {
		t.Fatalf("Reorder() error = %v", err)
	}
	if reordered.Items[0].ID != third.ID || reordered.Items[1].ID != firstID || reordered.Items[2].ID != secondID {
		t.Fatalf("reordered tasks = %#v", reordered.Items)
	}
	if _, err := taskService.Reorder(user.ID, &types.ReorderTasksRequest{ProjectID: project.Project.ID, TaskIDs: []int64{firstID, secondID}}); !errors.Is(err, logicerrors.ErrConflict) {
		t.Fatalf("incomplete Reorder() error = %v, want conflict", err)
	}
	afterFailure, err := gorm.G[taskmodel.Task](db).
		Where("project_id = ?", project.Project.ID).
		Order("sort_order ASC, id ASC").
		Find(ctx)
	if err != nil {
		t.Fatalf("list tasks after failed reorder: %v", err)
	}
	if afterFailure[0].ID != third.ID || afterFailure[1].ID != firstID || afterFailure[2].ID != secondID {
		t.Fatalf("tasks changed after failed reorder = %#v", afterFailure)
	}
}

func TestIntegrationProjectAndTaskHistoryIsolation(t *testing.T) {
	db := newIntegrationDB(t)
	ctx := context.Background()
	serviceContext := &svc.ServiceContext{DB: db}
	owner := createIntegrationUser(t, db, "history-owner@example.com")
	other := createIntegrationUser(t, db, "history-other@example.com")
	result := createIntegrationParseResult(t, db, owner.ID, true, json.RawMessage(`[{
		"title":"History task","priority":"medium"
	}]`))
	projectService := projectlogic.NewService(ctx, serviceContext)
	created, _, err := projectService.Create(owner.ID, &types.CreateProjectRequest{ParseResultID: result.ID, Name: "History"})
	if err != nil {
		t.Fatalf("Create project error = %v", err)
	}
	if _, err := projectService.Archive(owner.ID, created.Project.ID); err != nil {
		t.Fatalf("Archive project error = %v", err)
	}
	if err := projectService.Delete(owner.ID, created.Project.ID); err != nil {
		t.Fatalf("Delete project error = %v", err)
	}
	history, err := projectService.HistoryList(owner.ID, &types.HistoryProjectListRequest{})
	if err != nil || len(history.Items) != 1 || history.Items[0].Status != "deleted" {
		t.Fatalf("HistoryList() = %#v, %v", history, err)
	}
	tasks, err := tasklogic.NewService(ctx, serviceContext).HistoryList(owner.ID, created.Project.ID, &types.TaskListRequest{})
	if err != nil || len(tasks.Items) != 1 {
		t.Fatalf("History task List() = %#v, %v", tasks, err)
	}
	if _, err := projectService.HistoryGet(other.ID, created.Project.ID); !errors.Is(err, logicerrors.ErrNotFound) {
		t.Fatalf("other user HistoryGet() error = %v, want not found", err)
	}
	if _, err := tasklogic.NewService(ctx, serviceContext).HistoryList(other.ID, created.Project.ID, &types.TaskListRequest{}); !errors.Is(err, logicerrors.ErrNotFound) {
		t.Fatalf("other user History task List() error = %v, want not found", err)
	}
}
