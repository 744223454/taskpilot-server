package logic_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	logicerrors "github.com/744223454/taskpilot-server/internal/logic"
	documentlogic "github.com/744223454/taskpilot-server/internal/logic/document"
	parsejoblogic "github.com/744223454/taskpilot-server/internal/logic/parsejob"
	parseresultlogic "github.com/744223454/taskpilot-server/internal/logic/parseresult"
	"github.com/744223454/taskpilot-server/internal/svc"
	"github.com/744223454/taskpilot-server/internal/types"
	parseworker "github.com/744223454/taskpilot-server/internal/worker/parsejob"
	"github.com/744223454/taskpilot-server/model/documentmodel"
	"github.com/744223454/taskpilot-server/model/parsejobmodel"
	"github.com/744223454/taskpilot-server/model/parseresultmodel"
	"github.com/744223454/taskpilot-server/model/projectmodel"
	"github.com/744223454/taskpilot-server/model/taskmodel"
	"github.com/744223454/taskpilot-server/model/usermodel"
	"github.com/744223454/taskpilot-server/pkg/ai"
	cachepkg "github.com/744223454/taskpilot-server/pkg/cache"
	"github.com/744223454/taskpilot-server/pkg/database"
	"github.com/alicebob/miniredis/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func TestIntegrationConcurrentParseJobCreationAllowsOneActiveJob(t *testing.T) {
	db := newIntegrationDB(t)
	ctx := context.Background()
	serviceContext := &svc.ServiceContext{DB: db}
	user := createIntegrationUser(t, db, "concurrent@example.com")
	document, err := documentlogic.NewService(ctx, serviceContext).CreateText(user.ID, &types.CreateTextDocumentRequest{
		Title: "Concurrent parse test",
		Text:  "Only one active parse job may exist.",
	})
	if err != nil {
		t.Fatalf("CreateText() error = %v", err)
	}

	const requests = 8
	start := make(chan struct{})
	errorsChannel := make(chan error, requests)
	var waitGroup sync.WaitGroup
	for range requests {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			<-start
			_, createErr := parsejoblogic.NewService(ctx, serviceContext).Create(user.ID, &types.CreateParseJobRequest{DocumentID: document.ID})
			errorsChannel <- createErr
		}()
	}
	close(start)
	waitGroup.Wait()
	close(errorsChannel)

	successes := 0
	conflicts := 0
	for createErr := range errorsChannel {
		switch {
		case createErr == nil:
			successes++
		case errors.Is(createErr, logicerrors.ErrConflict):
			conflicts++
		default:
			t.Fatalf("unexpected Create() error = %v", createErr)
		}
	}
	if successes != 1 || conflicts != requests-1 {
		t.Fatalf("successes = %d, conflicts = %d; want 1 and %d", successes, conflicts, requests-1)
	}
}

func TestIntegrationDocumentSoftDeletePreservesProjectAndBlocksActiveJob(t *testing.T) {
	db := newIntegrationDB(t)
	ctx := context.Background()
	serviceContext := &svc.ServiceContext{DB: db}
	user := createIntegrationUser(t, db, "delete@example.com")
	document, err := documentlogic.NewService(ctx, serviceContext).CreateText(user.ID, &types.CreateTextDocumentRequest{
		Title: "Soft delete test",
		Text:  "Projects must survive source document deletion.",
	})
	if err != nil {
		t.Fatalf("CreateText() error = %v", err)
	}
	job, err := parsejoblogic.NewService(ctx, serviceContext).Create(user.ID, &types.CreateParseJobRequest{DocumentID: document.ID})
	if err != nil {
		t.Fatalf("Create parse job error = %v", err)
	}

	if err := documentlogic.NewService(ctx, serviceContext).Delete(user.ID, document.ID); !errors.Is(err, logicerrors.ErrConflict) {
		t.Fatalf("Delete() with active job error = %v, want conflict", err)
	}
	if _, err := gorm.G[parsejobmodel.ParseJob](db).
		Where("id = ?", job.ID).
		Set(clause.Assignments(map[string]any{"status": "success"})).
		Update(ctx); err != nil {
		t.Fatalf("mark parse job successful: %v", err)
	}

	result := parseresultmodel.ParseResult{
		UserID:          user.ID,
		DocumentID:      document.ID,
		ParseJobID:      job.ID,
		Title:           "Parsed result",
		Summary:         "Summary",
		Deliverables:    json.RawMessage(`[]`),
		KeyRequirements: json.RawMessage(`[]`),
		RiskWarnings:    json.RawMessage(`[]`),
		GeneratedTasks:  json.RawMessage(`[]`),
		Version:         1,
	}
	if err := gorm.G[parseresultmodel.ParseResult](db).Create(ctx, &result); err != nil {
		t.Fatalf("create parse result: %v", err)
	}
	project := projectmodel.Project{
		UserID:           user.ID,
		SourceDocumentID: document.ID,
		ParseResultID:    result.ID,
		Name:             "Preserved project",
		Status:           "active",
	}
	if err := gorm.G[projectmodel.Project](db).Create(ctx, &project); err != nil {
		t.Fatalf("create project: %v", err)
	}
	task := taskmodel.Task{
		ProjectID:  project.ID,
		UserID:     user.ID,
		Title:      "Preserved task",
		Status:     "todo",
		Priority:   "medium",
		SourceType: "manual",
	}
	if err := gorm.G[taskmodel.Task](db).Create(ctx, &task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	if err := documentlogic.NewService(ctx, serviceContext).Delete(user.ID, document.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := documentlogic.NewService(ctx, serviceContext).Get(user.ID, document.ID); !errors.Is(err, logicerrors.ErrNotFound) {
		t.Fatalf("Get() deleted document error = %v, want not found", err)
	}
	deletedDocument, err := gorm.G[documentmodel.Document](db).
		Scopes(func(statement *gorm.Statement) { statement.Unscoped = true }).
		Where("id = ?", document.ID).
		First(ctx)
	if err != nil || !deletedDocument.DeletedAt.Valid {
		t.Fatalf("soft-deleted document = %#v, error = %v", deletedDocument, err)
	}
	if _, err := gorm.G[projectmodel.Project](db).Where("id = ?", project.ID).First(ctx); err != nil {
		t.Fatalf("project was not preserved: %v", err)
	}
	if _, err := gorm.G[taskmodel.Task](db).Where("id = ?", task.ID).First(ctx); err != nil {
		t.Fatalf("task was not preserved: %v", err)
	}
}

func TestIntegrationParseWorkerAndResultEditingFlow(t *testing.T) {
	db := newIntegrationDB(t)
	redisServer, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer redisServer.Close()

	ctx := context.Background()
	queue := cachepkg.NewParseJobQueue(cachepkg.NewRedis(redisServer.Addr(), ""), "taskpilot:test:parse_jobs", "test-workers")
	serviceContext := &svc.ServiceContext{
		DB:        db,
		ParseJobs: queue,
		Parser: fakeParser{result: ai.ParsedDocument{
			Title:           "Parsed title",
			Summary:         "Parsed summary",
			Deliverables:    []string{"Report"},
			KeyRequirements: []string{"Use Go"},
			RiskWarnings:    []string{},
			GeneratedTasks: []ai.GeneratedTask{{
				Title:    "Write report",
				Priority: "high",
			}},
			Model: "fake-model",
		}},
	}
	user := createIntegrationUser(t, db, "worker@example.com")
	document, err := documentlogic.NewService(ctx, serviceContext).CreateText(user.ID, &types.CreateTextDocumentRequest{
		Title: "Worker parse test",
		Text:  "Please write a report using Go.",
	})
	if err != nil {
		t.Fatalf("CreateText() error = %v", err)
	}
	job, err := parsejoblogic.NewService(ctx, serviceContext).Create(user.ID, &types.CreateParseJobRequest{DocumentID: document.ID})
	if err != nil {
		t.Fatalf("Create parse job error = %v", err)
	}

	worker, err := parseworker.New(serviceContext)
	if err != nil {
		t.Fatalf("New worker error = %v", err)
	}
	if err := worker.ProcessJob(ctx, job.ID); err != nil {
		t.Fatalf("ProcessJob() error = %v", err)
	}
	if err := worker.ProcessJob(ctx, job.ID); err != nil {
		t.Fatalf("duplicate ProcessJob() error = %v", err)
	}

	storedJob, err := gorm.G[parsejobmodel.ParseJob](db).Where("id = ?", job.ID).First(ctx)
	if err != nil || storedJob.Status != "success" || storedJob.FinishedAt == nil {
		t.Fatalf("stored job = %#v, error = %v", storedJob, err)
	}
	resultCount, err := gorm.G[parseresultmodel.ParseResult](db).Where("parse_job_id = ?", job.ID).Count(ctx, "id")
	if err != nil || resultCount != 1 {
		t.Fatalf("result count = %d, error = %v", resultCount, err)
	}

	resultService := parseresultlogic.NewService(ctx, serviceContext)
	result, err := resultService.GetByJob(user.ID, job.ID)
	if err != nil {
		t.Fatalf("GetByJob() error = %v", err)
	}
	updated, err := resultService.Update(user.ID, result.ID, &types.UpdateParseResultRequest{
		Version:         result.Version,
		Title:           " Updated title ",
		Summary:         " Updated summary ",
		Deliverables:    []string{"Report", "Report"},
		KeyRequirements: []string{"Use Go"},
		RiskWarnings:    []string{},
		GeneratedTasks: []types.GeneratedTask{{
			Title: "Revise report",
		}},
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Version != result.Version+1 || updated.Title != "Updated title" || updated.GeneratedTasks[0].Priority != "medium" || len(updated.Deliverables) != 1 {
		t.Fatalf("updated result = %#v", updated)
	}
	if _, err := resultService.Update(user.ID, result.ID, &types.UpdateParseResultRequest{
		Version:         result.Version,
		Title:           "Stale",
		Summary:         "Stale",
		Deliverables:    []string{},
		KeyRequirements: []string{},
		RiskWarnings:    []string{},
		GeneratedTasks:  []types.GeneratedTask{},
	}); !errors.Is(err, logicerrors.ErrConflict) {
		t.Fatalf("stale Update() error = %v, want conflict", err)
	}

	confirmed, err := resultService.Confirm(user.ID, result.ID)
	if err != nil || !confirmed.IsConfirmed {
		t.Fatalf("Confirm() = %#v, %v", confirmed, err)
	}
	confirmedAgain, err := resultService.Confirm(user.ID, result.ID)
	if err != nil || !confirmedAgain.IsConfirmed {
		t.Fatalf("second Confirm() = %#v, %v", confirmedAgain, err)
	}
	if _, err := resultService.Update(user.ID, result.ID, &types.UpdateParseResultRequest{
		Version:         confirmed.Version,
		Title:           "No longer editable",
		Summary:         "No longer editable",
		Deliverables:    []string{},
		KeyRequirements: []string{},
		RiskWarnings:    []string{},
		GeneratedTasks:  []types.GeneratedTask{},
	}); !errors.Is(err, logicerrors.ErrInvalidState) {
		t.Fatalf("confirmed Update() error = %v, want invalid state", err)
	}

	otherUser := createIntegrationUser(t, db, "other-worker@example.com")
	if _, err := resultService.Get(otherUser.ID, result.ID); !errors.Is(err, logicerrors.ErrNotFound) {
		t.Fatalf("other user Get() error = %v, want not found", err)
	}
}

type fakeParser struct {
	result ai.ParsedDocument
	err    error
}

func (p fakeParser) Parse(context.Context, string) (ai.ParsedDocument, error) {
	return p.result, p.err
}

func newIntegrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TASKPILOT_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TASKPILOT_TEST_DATABASE_DSN is not set")
	}

	adminDB, err := database.NewPostgres(dsn)
	if err != nil {
		t.Fatalf("connect integration database: %v", err)
	}
	schema := "taskpilot_test_" + randomHex(t, 8)
	ctx := context.Background()
	if err := gorm.G[struct{}](adminDB).Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create integration schema: %v", err)
	}
	t.Cleanup(func() {
		_ = gorm.G[struct{}](adminDB).Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		if sqlDB, sqlErr := adminDB.DB(); sqlErr == nil {
			_ = sqlDB.Close()
		}
	})

	integrationURL, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse integration DSN: %v", err)
	}
	query := integrationURL.Query()
	query.Set("search_path", schema)
	integrationURL.RawQuery = query.Encode()
	db, err := database.NewPostgres(integrationURL.String())
	if err != nil {
		t.Fatalf("connect integration schema: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, sqlErr := db.DB(); sqlErr == nil {
			_ = sqlDB.Close()
		}
	})

	migration, err := os.ReadFile("../../scripts/migrate.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	for _, statement := range strings.Split(string(migration), ";") {
		statement = strings.TrimSpace(statement)
		if statement == "" {
			continue
		}
		if err := gorm.G[struct{}](db).Exec(ctx, statement); err != nil {
			t.Fatalf("apply migration statement: %v", err)
		}
	}
	return db
}

func createIntegrationUser(t *testing.T, db *gorm.DB, email string) usermodel.User {
	t.Helper()
	user := usermodel.User{Email: email, PasswordHash: "unused", Nickname: "integration", Status: 1}
	if err := gorm.G[usermodel.User](db).Create(context.Background(), &user); err != nil {
		t.Fatalf("create integration user: %v", err)
	}
	return user
}

func randomHex(t *testing.T, size int) string {
	t.Helper()
	random := make([]byte, size)
	if _, err := rand.Read(random); err != nil {
		t.Fatalf("generate random schema name: %v", err)
	}
	return hex.EncodeToString(random)
}
