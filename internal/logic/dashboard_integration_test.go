package logic_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	dashboardlogic "github.com/744223454/taskpilot-server/internal/logic/dashboard"
	projectlogic "github.com/744223454/taskpilot-server/internal/logic/project"
	"github.com/744223454/taskpilot-server/internal/svc"
	"github.com/744223454/taskpilot-server/internal/types"
	"github.com/744223454/taskpilot-server/model/documentmodel"
	"github.com/744223454/taskpilot-server/model/taskmodel"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func TestIntegrationDashboardStatsAndReminders(t *testing.T) {
	db := newIntegrationDB(t)
	ctx := context.Background()
	serviceContext := &svc.ServiceContext{DB: db}
	owner := createIntegrationUser(t, db, "dashboard-owner@example.com")
	other := createIntegrationUser(t, db, "dashboard-other@example.com")
	projectService := projectlogic.NewService(ctx, serviceContext)

	activeResult := createIntegrationParseResult(t, db, owner.ID, true, json.RawMessage(`[
		{"title":"Soon","priority":"high"},
		{"title":"Later","priority":"medium"},
		{"title":"Done","priority":"low"},
		{"title":"Overdue","priority":"high"},
		{"title":"Far away","priority":"low"}
	]`))
	active, _, err := projectService.Create(owner.ID, &types.CreateProjectRequest{ParseResultID: activeResult.ID, Name: "Active dashboard project"})
	if err != nil {
		t.Fatalf("create active project: %v", err)
	}
	now := time.Now()
	updates := []map[string]any{
		{"deadline": now.Add(36 * time.Hour)},
		{"deadline": now.Add(5 * 24 * time.Hour), "status": "doing"},
		{"deadline": now.Add(24 * time.Hour), "status": "done"},
		{"deadline": now.Add(-time.Hour)},
		{"deadline": now.Add(10 * 24 * time.Hour)},
	}
	for index, assignments := range updates {
		if _, err := gorm.G[taskmodel.Task](db).
			Where("id = ?", active.Tasks[index].ID).
			Set(clause.Assignments(assignments)).
			Update(ctx); err != nil {
			t.Fatalf("update active task %d: %v", index, err)
		}
	}

	archivedResult := createIntegrationParseResult(t, db, owner.ID, true, json.RawMessage(`[
		{"title":"Archived reminder","priority":"high"}
	]`))
	archived, _, err := projectService.Create(owner.ID, &types.CreateProjectRequest{ParseResultID: archivedResult.ID, Name: "Archived dashboard project"})
	if err != nil {
		t.Fatalf("create archived project: %v", err)
	}
	if _, err := gorm.G[taskmodel.Task](db).
		Where("id = ?", archived.Tasks[0].ID).
		Set(clause.Assignments(map[string]any{"deadline": now.Add(48 * time.Hour)})).
		Update(ctx); err != nil {
		t.Fatalf("update archived task deadline: %v", err)
	}
	if _, err := projectService.Archive(owner.ID, archived.Project.ID); err != nil {
		t.Fatalf("archive project: %v", err)
	}

	deletedTitle := "Deleted dashboard document"
	deletedContent := "This document should not be counted after soft deletion."
	deletedDocument := documentmodel.Document{
		UserID:     owner.ID,
		SourceType: "text",
		Title:      &deletedTitle,
		TextInput:  &deletedContent,
		Status:     "ready",
	}
	if err := gorm.G[documentmodel.Document](db).Create(ctx, &deletedDocument); err != nil {
		t.Fatalf("create deleted document: %v", err)
	}
	if _, err := gorm.G[documentmodel.Document](db).Where("id = ?", deletedDocument.ID).Delete(ctx); err != nil {
		t.Fatalf("soft delete document: %v", err)
	}

	otherResult := createIntegrationParseResult(t, db, other.ID, true, json.RawMessage(`[
		{"title":"Other user reminder","priority":"high"}
	]`))
	otherProject, _, err := projectService.Create(other.ID, &types.CreateProjectRequest{ParseResultID: otherResult.ID, Name: "Other user project"})
	if err != nil {
		t.Fatalf("create other user project: %v", err)
	}
	if _, err := gorm.G[taskmodel.Task](db).
		Where("id = ?", otherProject.Tasks[0].ID).
		Set(clause.Assignments(map[string]any{"deadline": now.Add(24 * time.Hour)})).
		Update(ctx); err != nil {
		t.Fatalf("update other user task deadline: %v", err)
	}

	service := dashboardlogic.NewService(ctx, serviceContext)
	stats, err := service.Stats(owner.ID)
	if err != nil {
		t.Fatalf("Stats() error = %v", err)
	}
	if stats.Documents != 2 || stats.ParseJobs != 2 || stats.ActiveProjects != 1 || stats.OpenTasks != 4 {
		t.Fatalf("Stats() = %#v", stats)
	}

	reminders, err := service.Reminders(owner.ID)
	if err != nil {
		t.Fatalf("Reminders() error = %v", err)
	}
	if len(reminders.Items) != 2 {
		t.Fatalf("Reminders() = %#v", reminders.Items)
	}
	if reminders.Items[0].Title != "Soon" || reminders.Items[0].Project != active.Project.Name || reminders.Items[0].ProjectID != active.Project.ID || reminders.Items[0].DaysLeft != 2 {
		t.Fatalf("first reminder = %#v", reminders.Items[0])
	}
	if reminders.Items[1].Title != "Later" || reminders.Items[1].DaysLeft != 5 {
		t.Fatalf("second reminder = %#v", reminders.Items[1])
	}
}
