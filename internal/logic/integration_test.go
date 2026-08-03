package logic_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	logicerrors "github.com/744223454/taskpilot-server/internal/logic"
	authlogic "github.com/744223454/taskpilot-server/internal/logic/auth"
	documentlogic "github.com/744223454/taskpilot-server/internal/logic/document"
	parsejoblogic "github.com/744223454/taskpilot-server/internal/logic/parsejob"
	parseresultlogic "github.com/744223454/taskpilot-server/internal/logic/parseresult"
	projectlogic "github.com/744223454/taskpilot-server/internal/logic/project"
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
	jwtauth "github.com/744223454/taskpilot-server/pkg/auth"
	cachepkg "github.com/744223454/taskpilot-server/pkg/cache"
	"github.com/744223454/taskpilot-server/pkg/database"
	"github.com/744223454/taskpilot-server/pkg/upload"
	"github.com/alicebob/miniredis/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func TestIntegrationPDFDocumentLifecycle(t *testing.T) {
	db := newIntegrationDB(t)
	ctx := context.Background()
	fileStore, err := upload.NewLocalFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalFileStore() error = %v", err)
	}
	serviceContext := &svc.ServiceContext{
		DB:           db,
		Files:        fileStore,
		PDFExtractor: fakePDFExtractor{result: upload.PDFResult{Text: "Extracted TaskPilot PDF document body.", PageCount: 3}},
	}
	serviceContext.Config.Upload.MaxFileBytes = 10 << 20
	user := createIntegrationUser(t, db, "pdf-document@example.com")

	document, err := documentlogic.NewService(ctx, serviceContext).CreatePDF(user.ID, &types.CreatePDFDocumentRequest{
		FileName: "project-brief.PDF",
	}, strings.NewReader("%PDF-1.4 fixture body"))
	if err != nil {
		t.Fatalf("CreatePDF() error = %v", err)
	}
	if document.SourceType != "pdf" || document.Title == nil || *document.Title != "project-brief" || document.FileURL == nil || document.PageCount == nil || *document.PageCount != 3 || document.FileSize == nil || *document.FileSize != 21 || document.Content == nil {
		t.Fatalf("CreatePDF() document = %#v", document)
	}
	stored, err := gorm.G[documentmodel.Document](db).Where("id = ?", document.ID).First(ctx)
	if err != nil || stored.RawText == nil || *stored.RawText != "Extracted TaskPilot PDF document body." || stored.TextInput != nil || stored.Status != "ready" {
		t.Fatalf("stored PDF document = %#v, error = %v", stored, err)
	}
	files, err := fileStore.List(ctx, "documents")
	if err != nil || len(files) != 1 || files[0].Key != *document.FileURL {
		t.Fatalf("stored PDF files = %#v, error = %v", files, err)
	}

	if err := documentlogic.NewService(ctx, serviceContext).Delete(user.ID, document.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	files, err = fileStore.List(ctx, "documents")
	if err != nil || len(files) != 0 {
		t.Fatalf("PDF files after delete = %#v, error = %v", files, err)
	}
}

func TestIntegrationUpdateCurrentUserRotatesCurrentSession(t *testing.T) {
	db := newIntegrationDB(t)
	redisServer, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer redisServer.Close()

	ctx := context.Background()
	user := createIntegrationUser(t, db, "profile-update@example.com")
	redisClient := cachepkg.NewRedis(redisServer.Addr(), "")
	defer redisClient.Close()
	store := cachepkg.NewRefreshSessionStore(redisClient)
	refreshToken, err := jwtauth.GenerateRefreshToken("")
	if err != nil {
		t.Fatalf("GenerateRefreshToken() error = %v", err)
	}
	expiresAt := time.Now().UTC().Add(time.Hour)
	if err := store.Create(ctx, jwtauth.RefreshSession{
		ID:        refreshToken.SessionID,
		UserID:    user.ID,
		Email:     user.Email,
		Nickname:  user.Nickname,
		ExpiresAt: expiresAt,
	}, refreshToken.Hash); err != nil {
		t.Fatalf("create refresh session: %v", err)
	}

	serviceContext := &svc.ServiceContext{
		DB:              db,
		JWT:             jwtauth.NewManager("integration-secret", 900),
		RefreshSessions: store,
	}
	serviceContext.Config.Auth.AccessExpire = 900
	serviceContext.Config.Auth.RefreshExpire = 3600
	nickname := " Updated profile "
	avatarURL := "https://example.com/avatar.png"
	session, err := authlogic.NewService(ctx, serviceContext).UpdateCurrentUser(user.ID, refreshToken.Raw, &types.UpdateUserRequest{
		Nickname:  types.OptionalString{Set: true, Value: &nickname},
		AvatarURL: types.OptionalString{Set: true, Value: &avatarURL},
	})
	if err != nil {
		t.Fatalf("UpdateCurrentUser() error = %v", err)
	}
	if session.Response.User.Nickname != "Updated profile" || session.Response.User.AvatarURL == nil || *session.Response.User.AvatarURL != avatarURL {
		t.Fatalf("updated profile = %#v", session.Response.User)
	}
	claims, err := serviceContext.JWT.ParseToken(session.Response.AccessToken)
	if err != nil || claims.Nickname != "Updated profile" {
		t.Fatalf("updated access claims = %#v, %v", claims, err)
	}
	if _, err := authlogic.NewService(ctx, serviceContext).Refresh(refreshToken.Raw); !errors.Is(err, authlogic.ErrRefreshTokenReused) {
		t.Fatalf("old refresh token error = %v, want replay rejection", err)
	}
}

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

func TestIntegrationCreateProjectFromConfirmedResultIsIdempotent(t *testing.T) {
	db := newIntegrationDB(t)
	ctx := context.Background()
	serviceContext := &svc.ServiceContext{DB: db}
	user := createIntegrationUser(t, db, "project@example.com")
	result := createIntegrationParseResult(t, db, user.ID, true, json.RawMessage(`[
		{"title":" First task ","description":" Prepare deliverable ","priority":"","deadline":null},
		{"title":"Second task","priority":"high","deadline":"2030-01-02T03:04:05Z"}
	]`))

	service := projectlogic.NewService(ctx, serviceContext)
	createdProject, created, err := service.Create(user.ID, &types.CreateProjectRequest{
		ParseResultID: result.ID,
		Name:          "  Launch project  ",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !created || createdProject.Project.Name != "Launch project" || createdProject.Project.Description == nil || *createdProject.Project.Description != result.Summary || createdProject.Project.Status != "active" {
		t.Fatalf("created project = %#v, created = %v", createdProject, created)
	}
	if len(createdProject.Tasks) != 2 {
		t.Fatalf("tasks = %#v", createdProject.Tasks)
	}
	firstTask := createdProject.Tasks[0]
	if firstTask.Title != "First task" || firstTask.Description == nil || *firstTask.Description != "Prepare deliverable" || firstTask.Priority != "medium" || firstTask.Status != "todo" || firstTask.SourceType != "ai" || firstTask.SortOrder != 0 || firstTask.SourceParseResultID == nil || *firstTask.SourceParseResultID != result.ID {
		t.Fatalf("first task = %#v", firstTask)
	}
	if createdProject.Tasks[1].Priority != "high" || createdProject.Tasks[1].SortOrder != 1 || createdProject.Tasks[1].Deadline == nil {
		t.Fatalf("second task = %#v", createdProject.Tasks[1])
	}

	existingProject, createdAgain, err := service.Create(user.ID, &types.CreateProjectRequest{
		ParseResultID: result.ID,
		Name:          "Ignored replacement name",
	})
	if err != nil {
		t.Fatalf("second Create() error = %v", err)
	}
	if createdAgain || existingProject.Project.ID != createdProject.Project.ID || existingProject.Project.Name != "Launch project" || len(existingProject.Tasks) != 2 || existingProject.Tasks[0].ID != createdProject.Tasks[0].ID {
		t.Fatalf("existing project = %#v, created = %v", existingProject, createdAgain)
	}

	projectCount, err := gorm.G[projectmodel.Project](db).Where("parse_result_id = ?", result.ID).Count(ctx, "id")
	if err != nil || projectCount != 1 {
		t.Fatalf("project count = %d, error = %v", projectCount, err)
	}
	taskCount, err := gorm.G[taskmodel.Task](db).Where("project_id = ?", createdProject.Project.ID).Count(ctx, "id")
	if err != nil || taskCount != 2 {
		t.Fatalf("task count = %d, error = %v", taskCount, err)
	}

	otherUser := createIntegrationUser(t, db, "project-other@example.com")
	if _, _, err := service.Create(otherUser.ID, &types.CreateProjectRequest{ParseResultID: result.ID, Name: "Forbidden"}); !errors.Is(err, logicerrors.ErrNotFound) {
		t.Fatalf("other user Create() error = %v, want not found", err)
	}
}

func TestIntegrationCreateProjectAllowsEmptyTasksAndRollsBackInvalidState(t *testing.T) {
	db := newIntegrationDB(t)
	ctx := context.Background()
	serviceContext := &svc.ServiceContext{DB: db}
	user := createIntegrationUser(t, db, "project-state@example.com")
	service := projectlogic.NewService(ctx, serviceContext)

	emptyResult := createIntegrationParseResult(t, db, user.ID, true, json.RawMessage(`[]`))
	emptyProject, created, err := service.Create(user.ID, &types.CreateProjectRequest{ParseResultID: emptyResult.ID, Name: "Empty project"})
	if err != nil || !created || emptyProject.Tasks == nil || len(emptyProject.Tasks) != 0 {
		t.Fatalf("empty Create() = %#v, %v, %v", emptyProject, created, err)
	}

	unconfirmed := createIntegrationParseResult(t, db, user.ID, false, json.RawMessage(`[]`))
	if _, _, err := service.Create(user.ID, &types.CreateProjectRequest{ParseResultID: unconfirmed.ID, Name: "Unconfirmed"}); !errors.Is(err, logicerrors.ErrInvalidState) {
		t.Fatalf("unconfirmed Create() error = %v, want invalid state", err)
	}

	invalid := createIntegrationParseResult(t, db, user.ID, true, json.RawMessage(`[{"title":"Broken","priority":"urgent"}]`))
	if _, _, err := service.Create(user.ID, &types.CreateProjectRequest{ParseResultID: invalid.ID, Name: "Invalid"}); !errors.Is(err, logicerrors.ErrInvalidState) {
		t.Fatalf("invalid Create() error = %v, want invalid state", err)
	}
	invalidProjectCount, err := gorm.G[projectmodel.Project](db).Where("parse_result_id IN (?, ?)", unconfirmed.ID, invalid.ID).Count(ctx, "id")
	if err != nil || invalidProjectCount != 0 {
		t.Fatalf("invalid project count = %d, error = %v", invalidProjectCount, err)
	}
}

func TestIntegrationConcurrentProjectCreationReturnsOneProject(t *testing.T) {
	db := newIntegrationDB(t)
	ctx := context.Background()
	serviceContext := &svc.ServiceContext{DB: db}
	user := createIntegrationUser(t, db, "project-concurrent@example.com")
	result := createIntegrationParseResult(t, db, user.ID, true, json.RawMessage(`[{"title":"Only task","priority":"medium"}]`))

	const requests = 8
	type createResult struct {
		projectID int64
		created   bool
		err       error
	}
	start := make(chan struct{})
	results := make(chan createResult, requests)
	var waitGroup sync.WaitGroup
	for index := range requests {
		waitGroup.Add(1)
		go func(requestIndex int) {
			defer waitGroup.Done()
			<-start
			response, created, err := projectlogic.NewService(ctx, serviceContext).Create(user.ID, &types.CreateProjectRequest{
				ParseResultID: result.ID,
				Name:          fmt.Sprintf("Project %d", requestIndex),
			})
			projectID := int64(0)
			if response != nil {
				projectID = response.Project.ID
			}
			results <- createResult{projectID: projectID, created: created, err: err}
		}(index)
	}
	close(start)
	waitGroup.Wait()
	close(results)

	createdCount := 0
	projectID := int64(0)
	for result := range results {
		if result.err != nil {
			t.Fatalf("concurrent Create() error = %v", result.err)
		}
		if result.created {
			createdCount++
		}
		if projectID == 0 {
			projectID = result.projectID
		}
		if result.projectID != projectID {
			t.Fatalf("project ID = %d, want %d", result.projectID, projectID)
		}
	}
	if createdCount != 1 {
		t.Fatalf("created count = %d, want 1", createdCount)
	}
	projectCount, err := gorm.G[projectmodel.Project](db).Where("parse_result_id = ?", result.ID).Count(ctx, "id")
	if err != nil || projectCount != 1 {
		t.Fatalf("project count = %d, error = %v", projectCount, err)
	}
	taskCount, err := gorm.G[taskmodel.Task](db).Where("project_id = ?", projectID).Count(ctx, "id")
	if err != nil || taskCount != 1 {
		t.Fatalf("task count = %d, error = %v", taskCount, err)
	}
}

type fakeParser struct {
	result ai.ParsedDocument
	err    error
}

type fakePDFExtractor struct {
	result upload.PDFResult
}

func (p fakePDFExtractor) Extract(context.Context, string) (upload.PDFResult, error) {
	return p.result, nil
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

func createIntegrationParseResult(t *testing.T, db *gorm.DB, userID int64, confirmed bool, generatedTasks json.RawMessage) parseresultmodel.ParseResult {
	t.Helper()
	ctx := context.Background()
	title := "Project source"
	content := "Create a project from this document."
	document := documentmodel.Document{
		UserID:     userID,
		SourceType: "text",
		Title:      &title,
		RawText:    &content,
		TextInput:  &content,
		Status:     "ready",
	}
	if err := gorm.G[documentmodel.Document](db).Create(ctx, &document); err != nil {
		t.Fatalf("create project source document: %v", err)
	}
	job := parsejobmodel.ParseJob{
		UserID:     userID,
		DocumentID: document.ID,
		JobType:    "ai_parse",
		Status:     "success",
	}
	if err := gorm.G[parsejobmodel.ParseJob](db).Create(ctx, &job); err != nil {
		t.Fatalf("create project source job: %v", err)
	}
	result := parseresultmodel.ParseResult{
		UserID:          userID,
		DocumentID:      document.ID,
		ParseJobID:      job.ID,
		Title:           title,
		Summary:         "Project summary",
		Deliverables:    json.RawMessage(`[]`),
		KeyRequirements: json.RawMessage(`[]`),
		RiskWarnings:    json.RawMessage(`[]`),
		GeneratedTasks:  generatedTasks,
		Version:         1,
		IsConfirmed:     confirmed,
	}
	if err := gorm.G[parseresultmodel.ParseResult](db).Create(ctx, &result); err != nil {
		t.Fatalf("create project source result: %v", err)
	}
	return result
}

func randomHex(t *testing.T, size int) string {
	t.Helper()
	random := make([]byte, size)
	if _, err := rand.Read(random); err != nil {
		t.Fatalf("generate random schema name: %v", err)
	}
	return hex.EncodeToString(random)
}
