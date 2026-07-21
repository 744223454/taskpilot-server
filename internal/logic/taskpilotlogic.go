package logic

import (
	"context"
	"fmt"

	"github.com/744223454/taskpilot-server/internal/svc"
	"github.com/744223454/taskpilot-server/internal/types"
)

type TaskpilotLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewTaskpilotLogic(ctx context.Context, svcCtx *svc.ServiceContext) *TaskpilotLogic {
	return &TaskpilotLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *TaskpilotLogic) Taskpilot(req *types.Request) (resp *types.Response, err error) {
	resp = &types.Response{
		Message: fmt.Sprintf("hello %s", req.Name),
	}

	return resp, nil
}
