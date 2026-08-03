package dashboard

import (
	"context"
	"fmt"
	"math"
	"time"

	logicerrors "github.com/744223454/taskpilot-server/internal/logic"
	"github.com/744223454/taskpilot-server/internal/svc"
	"github.com/744223454/taskpilot-server/internal/types"
	"gorm.io/gorm"
)

const (
	reminderLimit  = 10
	reminderWindow = 7 * 24 * time.Hour
)

type Service struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	now    func() time.Time
}

type statsRow struct {
	Documents      int64 `gorm:"column:documents"`
	ParseJobs      int64 `gorm:"column:parse_jobs"`
	ActiveProjects int64 `gorm:"column:active_projects"`
	OpenTasks      int64 `gorm:"column:open_tasks"`
}

type reminderRow struct {
	ID        int64     `gorm:"column:id"`
	ProjectID int64     `gorm:"column:project_id"`
	Title     string    `gorm:"column:title"`
	Project   string    `gorm:"column:project"`
	Deadline  time.Time `gorm:"column:deadline"`
}

func NewService(ctx context.Context, svcCtx *svc.ServiceContext) *Service {
	return &Service{ctx: ctx, svcCtx: svcCtx, now: time.Now}
}

func (s *Service) Stats(userID int64) (*types.DashboardStatsResponse, error) {
	if err := s.validate(userID); err != nil {
		return nil, err
	}

	row, err := gorm.G[statsRow](s.svcCtx.DB).Raw(`
		SELECT
			(SELECT COUNT(*) FROM documents WHERE user_id = ? AND deleted_at IS NULL) AS documents,
			(SELECT COUNT(*) FROM parse_jobs WHERE user_id = ?) AS parse_jobs,
			(SELECT COUNT(*) FROM projects WHERE user_id = ? AND status = 'active') AS active_projects,
			(SELECT COUNT(*)
			 FROM tasks AS task
			 JOIN projects AS project ON project.id = task.project_id
			 WHERE task.user_id = ?
			   AND project.user_id = task.user_id
			   AND project.status = 'active'
			   AND task.status IN ('todo', 'doing')) AS open_tasks
	`, userID, userID, userID, userID).First(s.ctx)
	if err != nil {
		return nil, fmt.Errorf("query dashboard stats: %w", err)
	}

	return &types.DashboardStatsResponse{
		Documents:      row.Documents,
		ParseJobs:      row.ParseJobs,
		ActiveProjects: row.ActiveProjects,
		OpenTasks:      row.OpenTasks,
	}, nil
}

func (s *Service) Reminders(userID int64) (*types.DashboardReminderListResponse, error) {
	if err := s.validate(userID); err != nil {
		return nil, err
	}

	now := s.now()
	rows, err := gorm.G[reminderRow](s.svcCtx.DB).Raw(`
		SELECT
			task.id,
			task.project_id,
			task.title,
			project.name AS project,
			task.deadline
		FROM tasks AS task
		JOIN projects AS project ON project.id = task.project_id
		WHERE task.user_id = ?
		  AND project.user_id = task.user_id
		  AND project.status = 'active'
		  AND task.status IN ('todo', 'doing')
		  AND task.deadline >= ?
		  AND task.deadline < ?
		ORDER BY task.deadline ASC, task.id ASC
		LIMIT ?
	`, userID, now, now.Add(reminderWindow), reminderLimit).Find(s.ctx)
	if err != nil {
		return nil, fmt.Errorf("query dashboard reminders: %w", err)
	}

	items := make([]types.DashboardReminderResponse, len(rows))
	for index, row := range rows {
		items[index] = types.DashboardReminderResponse{
			ID:        row.ID,
			ProjectID: row.ProjectID,
			Title:     row.Title,
			Project:   row.Project,
			Deadline:  row.Deadline,
			DaysLeft:  daysLeft(now, row.Deadline),
		}
	}

	return &types.DashboardReminderListResponse{Items: items}, nil
}

func (s *Service) validate(userID int64) error {
	if userID <= 0 {
		return logicerrors.ErrInvalidInput
	}
	if s.svcCtx == nil || s.svcCtx.DB == nil {
		return logicerrors.ErrDatabaseUnavailable
	}
	return nil
}

func daysLeft(now, deadline time.Time) int {
	remaining := deadline.Sub(now)
	if remaining <= 0 {
		return 0
	}
	return int(math.Ceil(float64(remaining) / float64(24*time.Hour)))
}
