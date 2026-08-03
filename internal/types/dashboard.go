package types

import "time"

type DashboardStatsResponse struct {
	Documents      int64 `json:"documents"`
	ParseJobs      int64 `json:"parse_jobs"`
	ActiveProjects int64 `json:"active_projects"`
	OpenTasks      int64 `json:"open_tasks"`
}

type DashboardReminderResponse struct {
	ID        int64     `json:"id"`
	ProjectID int64     `json:"project_id"`
	Title     string    `json:"title"`
	Project   string    `json:"project"`
	Deadline  time.Time `json:"deadline"`
	DaysLeft  int       `json:"days_left"`
}

type DashboardReminderListResponse struct {
	Items []DashboardReminderResponse `json:"items"`
}
