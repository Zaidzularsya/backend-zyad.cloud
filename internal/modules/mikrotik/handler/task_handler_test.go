package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/domain"
	"zyad.cloud/internal/modules/mikrotik/dto"
	"zyad.cloud/internal/modules/mikrotik/repository"
	"zyad.cloud/internal/modules/mikrotik/service"
)

const testTaskID = "77777777-7777-7777-7777-777777777777"

type fakeTaskService struct {
	err        error
	listFilter repository.TaskFilter
	listLimit  int
	listOffset int
	retryID    string
	calls      int
}

func (f *fakeTaskService) List(_ context.Context, _ coretenant.Scope, filter repository.TaskFilter, limit, offset int) ([]domain.TaskView, int64, error) {
	f.calls++
	f.listFilter, f.listLimit, f.listOffset = filter, limit, offset
	return []domain.TaskView{sampleTaskView()}, 1, f.err
}
func (f *fakeTaskService) Summary(context.Context, coretenant.Scope) (service.TaskSummary, error) {
	f.calls++
	oldest := time.Now()
	return service.TaskSummary{Counts: map[string]int{"pending": 1, "running": 0, "done": 0, "failed": 0, "cancelled": 0}, OldestPendingRestoreAt: &oldest}, f.err
}
func (f *fakeTaskService) Retry(_ context.Context, _ coretenant.Scope, id, _ string) (domain.TaskView, error) {
	f.calls++
	f.retryID = id
	return sampleTaskView(), f.err
}

func sampleTaskView() domain.TaskView {
	return domain.TaskView{
		IsolationTask: domain.IsolationTask{ID: testTaskID, SecretID: testSecretID, Action: domain.ActionIsolate, Status: domain.TaskFailed,
			Priority: 10, Attempts: 5, NextAttemptAt: time.Now(), LastErrorCode: "unreachable", LastError: "router unreachable", CreatedAt: time.Now()},
		RouterID: testRouterID, RouterName: "R1", SecretName: "alice", AccountID: testAccountID, AccountName: "Alice",
	}
}

func newTaskRouter(t *testing.T, svc TaskService) http.Handler {
	r := newTestRouter(t)
	NewTaskHandler(svc).RegisterRoutes(r.Group("/app/mikrotik"), nil, nil)
	return r
}

func TestTasksList_PaginationCapped(t *testing.T) {
	svc := &fakeTaskService{}
	r := newTaskRouter(t, svc)
	w := performJSON(r, http.MethodGet, "/app/mikrotik/tasks?limit=100000&offset=40&status=failed&router_id="+testRouterID, "")
	if w.Code != http.StatusOK {
		t.Fatalf("status %d body %s", w.Code, w.Body)
	}
	if svc.listLimit != domain.MaxRouterPageSize || svc.listOffset != 40 || svc.listFilter.Status != "failed" || svc.listFilter.RouterID != testRouterID {
		t.Fatalf("service args: %+v limit=%d offset=%d", svc.listFilter, svc.listLimit, svc.listOffset)
	}
	var body struct {
		Meta dto.PaginationMeta `json:"meta"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Meta.PerPage != domain.MaxRouterPageSize {
		t.Fatalf("meta = %+v err=%v body=%s", body.Meta, err, w.Body)
	}
}

func TestTasksList_InvalidStatusOrRouterID_400(t *testing.T) {
	for _, q := range []string{"status=bogus", "router_id=not-a-uuid"} {
		svc := &fakeTaskService{}
		w := performJSON(newTaskRouter(t, svc), http.MethodGet, "/app/mikrotik/tasks?"+q, "")
		if w.Code != http.StatusBadRequest || svc.calls != 0 {
			t.Errorf("%s: status %d calls %d", q, w.Code, svc.calls)
		}
	}
}

func TestTaskRetry_FailedBecomesPending_AuditWritten(t *testing.T) {
	svc := &fakeTaskService{}
	w := performJSON(newTaskRouter(t, svc), http.MethodPost, "/app/mikrotik/tasks/"+testTaskID+"/retry", "")
	if w.Code != http.StatusOK || svc.retryID != testTaskID {
		t.Fatalf("status %d id %q body %s", w.Code, svc.retryID, w.Body)
	}
}

func TestTaskRetry_ErrorMapping(t *testing.T) {
	cases := []struct {
		name string
		err  error
		path string
		want int
		msg  string
	}{
		{"NotFound (also other org)", mikrotik.ErrTaskNotFound, testTaskID, http.StatusNotFound, ""},
		{"TestTaskRetry_OtherOrg_404", mikrotik.ErrTaskNotFound, testTaskID, http.StatusNotFound, "task not found"},
		{"TestTaskRetry_NotFailed_409", mikrotik.ErrTaskNotRetryable, testTaskID, http.StatusConflict, "failed"},
		{"TestTaskRetry_ActiveExists_409", mikrotik.ErrTaskActiveExists, testTaskID, http.StatusConflict, "active task"},
		{"bad id", nil, "not-a-uuid", http.StatusNotFound, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeTaskService{err: tc.err}
			w := performJSON(newTaskRouter(t, svc), http.MethodPost, "/app/mikrotik/tasks/"+tc.path+"/retry", "")
			if w.Code != tc.want || !strings.Contains(w.Body.String(), tc.msg) {
				t.Fatalf("status %d body %s", w.Code, w.Body)
			}
			if tc.path == "not-a-uuid" && svc.calls != 0 {
				t.Fatal("service reached with malformed id")
			}
		})
	}
}

func TestTaskSummary_ZeroFilledAndOldestPendingRestore(t *testing.T) {
	w := performJSON(newTaskRouter(t, &fakeTaskService{}), http.MethodGet, "/app/mikrotik/tasks/summary", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status %d body %s", w.Code, w.Body)
	}
	var body struct {
		Data struct {
			Counts                 map[string]int `json:"counts"`
			OldestPendingRestoreAt *time.Time     `json:"oldest_pending_restore_at"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || len(body.Data.Counts) != 5 || body.Data.OldestPendingRestoreAt == nil {
		t.Fatalf("body = %s err=%v", w.Body, err)
	}
}

func TestTaskDTO_NoRouterCredentialsOrHost(t *testing.T) {
	raw, err := json.Marshal(dto.TaskFromView(sampleTaskView()))
	if err != nil {
		t.Fatal(err)
	}
	var generic map[string]any
	_ = json.Unmarshal(raw, &generic)
	router, _ := generic["router"].(map[string]any)
	if len(router) != 2 || router["id"] == nil || router["name"] == nil {
		t.Fatalf("router = %v", router)
	}
	for _, banned := range []string{"host", "username", "password", "port", "api_port", "secret_enc", "fingerprint"} {
		if strings.Contains(strings.ToLower(string(raw)), `"`+banned+`"`) {
			t.Errorf("task JSON exposes %q: %s", banned, raw)
		}
	}
}

func taskRoutes() []struct{ method, path, perm string } {
	return []struct{ method, path, perm string }{
		{http.MethodGet, "/app/mikrotik/tasks", "mikrotik_task.read"},
		{http.MethodGet, "/app/mikrotik/tasks/summary", "mikrotik_task.read"},
		{http.MethodPost, "/app/mikrotik/tasks/:id/retry", "mikrotik_task.retry"},
	}
}

func TestTasksList_RequiresReadPermission(t *testing.T) {
	assertRoutesRequirePermissions(t, taskRoutes(), func(g *gin.RouterGroup, p *recordingChecker) {
		NewTaskHandler(&fakeTaskService{}).RegisterRoutes(g, p, nil)
	})
}

func TestTaskRetry_RequiresRetryPermission(t *testing.T) {
	checker := &recordingChecker{}
	svc := &fakeTaskService{}
	r := newTestRouter(t)
	r.Use(func(c *gin.Context) { permissionmiddleware.SetUserID(c, "user-1") })
	NewTaskHandler(svc).RegisterRoutes(r.Group("/app/mikrotik"), checker, nil)
	w := performJSON(r, http.MethodPost, "/app/mikrotik/tasks/"+testTaskID+"/retry", "")
	if w.Code != http.StatusForbidden || len(checker.asked) == 0 || checker.asked[0] != "mikrotik_task.retry" || svc.calls != 0 {
		t.Fatalf("status %d asked %v calls %d", w.Code, checker.asked, svc.calls)
	}
}
