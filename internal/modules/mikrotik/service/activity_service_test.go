package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik/repository"
)

type fakeActivityStore struct {
	rows  []repository.ActivityRow
	total int64
	err   error
}

func (f *fakeActivityStore) ListActivity(context.Context, coretenant.Scope, []string, int, int) ([]repository.ActivityRow, int64, error) {
	return f.rows, f.total, f.err
}

type fakeActivityNames struct {
	names map[string]string
	err   error
	asked []string
}

func (f *fakeActivityNames) Names(_ context.Context, _ coretenant.Scope, ids []string) (map[string]string, error) {
	f.asked = ids
	return f.names, f.err
}

const actAcct = "aaaaaaaa-1111-1111-1111-111111111111"

func TestActivity_MessageFromMetadata(t *testing.T) {
	good := map[string]map[string]any{
		"isolation.isolated":         {"secret_id": "x"},
		"isolation.restored":         {},
		"task.failed":                {"action": "isolate", "error_code": "unreachable"},
		"task.cancelled":             {"action": "restore"},
		"task.retried":               {"action": "isolate"},
		"control.mode_changed":       {"from": "off", "to": "pilot"},
		"control.grace_days_changed": {"from": 3, "to": float64(5)},
		"control.restore_all":        {"enqueued": float64(4), "skipped": float64(1), "aborted": true},
		"pilot.added":                {},
		"pilot.removed":              {},
		"pilot.test_started":         {},
		"pilot.test_stopped":         {},
		"pilot.test_failed":          {"reason": "no_secret"},
		"customer_policy.exempt_set": {"exempt": true, "reason": "rahasia", "exempted_until": "2026-12-31"},
	}
	if len(good) != len(ActivityEvents) {
		t.Fatalf("test covers %d events, allow-list has %d", len(good), len(ActivityEvents))
	}
	for _, ev := range ActivityEvents {
		md, ok := good[ev]
		if !ok {
			t.Fatalf("event %s not covered", ev)
		}
		if m := activityMessage(ev, md, "Budi"); m == "" || strings.Contains(m, "rahasia") {
			t.Errorf("%s: message %q", ev, m)
		}
		// Wrong types, nil and empty metadata must not panic and still say something.
		for _, bad := range []map[string]any{nil, {}, {"from": []int{1}, "to": map[string]any{}, "enqueued": "x", "exempt": "yes", "action": 5, "error_code": 1, "exempted_until": 7, "reason": struct{}{}}} {
			if m := activityMessage(ev, bad, ""); m == "" {
				t.Errorf("%s: empty message for %v", ev, bad)
			}
		}
	}
	if m := activityMessage("isolation.isolated", nil, "Budi"); m != "Pelanggan Budi diisolir" {
		t.Errorf("isolated = %q", m)
	}
	if m := activityMessage("control.restore_all", map[string]any{"enqueued": 4, "skipped": 1, "aborted": true}, ""); !strings.Contains(m, "4 dijadwalkan") || !strings.Contains(m, "dihentikan sebagian") {
		t.Errorf("restore_all = %q", m)
	}
	if m := activityMessage("customer_policy.exempt_set", map[string]any{"exempt": false}, "Budi"); m != "Pengecualian dicabut untuk Budi" {
		t.Errorf("exempt revoked = %q", m)
	}
	if m := activityMessage("unknown.event", nil, ""); !strings.Contains(m, "unknown.event") {
		t.Errorf("unknown = %q", m)
	}
	// Free text with odd characters is not echoed.
	if m := activityMessage("task.failed", map[string]any{"error_code": "pass=hunter2 host 10.0.0.1"}, ""); strings.Contains(m, "hunter2") || strings.Contains(m, "10.0.0.1") {
		t.Errorf("unsafe code echoed: %q", m)
	}
}

func TestActivity_NamesFailureDoesNotFailEndpoint(t *testing.T) {
	store := &fakeActivityStore{total: 1, rows: []repository.ActivityRow{{
		ID: "1", At: time.Now(), Event: "pilot.added", TargetID: actAcct, Metadata: map[string]any{"account_id": actAcct},
	}}}
	svc := NewActivityService(store, &fakeActivityNames{err: errors.New("db down")})
	items, total, err := svc.List(context.Background(), coretenant.Scope{}, 50, 0)
	if err != nil || total != 1 || len(items) != 1 {
		t.Fatalf("items=%v total=%d err=%v", items, total, err)
	}
	if items[0].AccountID != actAcct || items[0].AccountName != "" || items[0].Message == "" {
		t.Fatalf("item = %+v", items[0])
	}
}

func TestActivity_NamesResolvedBatchedAndClamped(t *testing.T) {
	names := &fakeActivityNames{names: map[string]string{actAcct: "Budi"}}
	store := &fakeActivityStore{total: 2, rows: []repository.ActivityRow{
		{ID: "1", Event: "isolation.isolated", Metadata: map[string]any{}, SecretName: "pppoe-budi", SecretAccountID: actAcct},
		{ID: "2", Event: "pilot.added", Metadata: map[string]any{"account_id": actAcct}},
	}}
	items, _, err := NewActivityService(store, names).List(context.Background(), coretenant.Scope{}, 500, -3)
	if err != nil || len(names.asked) != 1 {
		t.Fatalf("asked=%v err=%v", names.asked, err)
	}
	if items[0].AccountName != "Budi" || items[0].SecretName != "pppoe-budi" || items[0].Message != "Pelanggan Budi diisolir" {
		t.Fatalf("item0 = %+v", items[0])
	}
}
