package service

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik/repository"
)

// ActivityEvents is the allow-list of audit events shown in the control
// center log. Router/secret management events are deliberately left out.
var ActivityEvents = []string{
	"isolation.isolated", "isolation.restored",
	"task.failed", "task.cancelled", "task.retried",
	"control.mode_changed", "control.grace_days_changed", "control.restore_all",
	"pilot.added", "pilot.removed", "pilot.test_started", "pilot.test_stopped", "pilot.test_failed",
	"customer_policy.exempt_set",
}

const (
	defaultActivityLimit = 50
	maxActivityLimit     = 100
)

// ActivityStore is the subset of repository.ActivityRepository the service uses.
type ActivityStore interface {
	ListActivity(ctx context.Context, scope coretenant.Scope, events []string, limit, offset int) ([]repository.ActivityRow, int64, error)
}

// ActivityNames resolves customer account names in one batch (account id ->
// name). Implemented in internal/app so this module does not import receivable.
type ActivityNames interface {
	Names(ctx context.Context, scope coretenant.Scope, ids []string) (map[string]string, error)
}

// ActivityItem is one log entry with its Indonesian message.
type ActivityItem struct {
	ID          string
	At          time.Time
	Event       string
	ActorUserID string
	AccountID   string
	AccountName string
	SecretName  string
	Message     string
}

type ActivityService struct {
	store ActivityStore
	names ActivityNames
}

func NewActivityService(store ActivityStore, names ActivityNames) *ActivityService {
	return &ActivityService{store: store, names: names}
}

var activityUUID = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// List returns the newest entries first. limit defaults to 50 and is capped at
// 100; a failing name lookup only leaves the names empty.
func (s *ActivityService) List(ctx context.Context, scope coretenant.Scope, limit, offset int) ([]ActivityItem, int64, error) {
	if limit <= 0 {
		limit = defaultActivityLimit
	}
	if limit > maxActivityLimit {
		limit = maxActivityLimit
	}
	if offset < 0 {
		offset = 0
	}
	rows, total, err := s.store.ListActivity(ctx, scope, ActivityEvents, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	accountIDs := make([]string, len(rows))
	var wanted []string
	seen := map[string]bool{}
	for i, r := range rows {
		id := mdString(r.Metadata, "account_id")
		if id == "" {
			id = r.SecretAccountID
		}
		if !activityUUID.MatchString(id) {
			id = ""
		}
		accountIDs[i] = id
		if id != "" && !seen[id] {
			seen[id] = true
			wanted = append(wanted, id)
		}
	}
	var names map[string]string
	if s.names != nil && len(wanted) > 0 {
		if names, err = s.names.Names(ctx, scope, wanted); err != nil {
			slog.WarnContext(ctx, "mikrotik: activity name lookup failed", "error", scrub(err.Error()))
			names = nil
		}
	}
	items := make([]ActivityItem, 0, len(rows))
	for i, r := range rows {
		it := ActivityItem{
			ID: r.ID, At: r.At, Event: r.Event, ActorUserID: r.ActorUserID,
			AccountID: accountIDs[i], AccountName: names[accountIDs[i]],
			SecretName: r.SecretName,
		}
		if it.SecretName == "" {
			it.SecretName = safeText(mdString(r.Metadata, "secret_name"), 120)
			if it.SecretName == "" {
				it.SecretName = safeText(mdString(r.Metadata, "secret"), 120)
			}
		}
		subject := it.AccountName
		if subject == "" {
			subject = it.SecretName
		}
		it.Message = activityMessage(r.Event, r.Metadata, subject)
		items = append(items, it)
	}
	return items, total, nil
}

func mdString(md map[string]any, key string) string {
	if v, ok := md[key].(string); ok {
		return v
	}
	return ""
}

func mdBool(md map[string]any, key string) (value, ok bool) {
	value, ok = md[key].(bool)
	return
}

// mdInt accepts the numeric shapes a jsonb round trip or an in-process map yields.
func mdInt(md map[string]any, key string) (int, bool) {
	switch n := md[key].(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	}
	return 0, false
}

var tokenPattern = regexp.MustCompile(`^[A-Za-z0-9_.\-]{1,40}$`)

// codeToken returns v only when it is a short code-like value (modes, error codes);
// anything else could be free text and is dropped.
func codeToken(v string) string {
	if tokenPattern.MatchString(v) {
		return v
	}
	return ""
}

// safeText trims names to a printable, bounded string.
func safeText(v string, max int) string {
	r := []rune(v)
	if len(r) > max {
		r = r[:max]
	}
	out := make([]rune, 0, len(r))
	for _, c := range r {
		if c >= ' ' && c != 0x7f {
			out = append(out, c)
		}
	}
	return string(out)
}

func actionWord(md map[string]any) string {
	switch mdString(md, "action") {
	case ActionIsolate:
		return "isolir"
	case ActionRestore:
		return "pemulihan"
	}
	return ""
}

func customer(name string) string {
	if name = safeText(name, 120); name != "" {
		return "Pelanggan " + name
	}
	return "Pelanggan"
}

// activityMessage builds the Indonesian line for an audit row. name is the
// customer (or secret) name when known. It never fails: missing or wrongly
// typed metadata falls back to a generic sentence, and only code-like values
// (never free text, reasons, hosts or credentials) from metadata are echoed.
func activityMessage(event string, md map[string]any, name string) string {
	switch event {
	case "isolation.isolated":
		return customer(name) + " diisolir"
	case "isolation.restored":
		return customer(name) + " dipulihkan"
	case "task.failed":
		msg := "Tugas gagal"
		if a := actionWord(md); a != "" {
			msg = "Tugas " + a + " gagal"
		}
		if code := codeToken(mdString(md, "error_code")); code != "" {
			msg += ": " + code
		}
		return msg
	case "task.cancelled":
		if a := actionWord(md); a != "" {
			return "Tugas " + a + " dibatalkan"
		}
		return "Tugas dibatalkan"
	case "task.retried":
		if a := actionWord(md); a != "" {
			return "Tugas " + a + " dicoba ulang"
		}
		return "Tugas dicoba ulang"
	case "control.mode_changed":
		from, to := codeToken(mdString(md, "from")), codeToken(mdString(md, "to"))
		if from != "" && to != "" {
			return fmt.Sprintf("Mode isolir diubah dari %s ke %s", from, to)
		}
		if to != "" {
			return "Mode isolir diubah ke " + to
		}
		return "Mode isolir diubah"
	case "control.grace_days_changed":
		if n, ok := mdInt(md, "to"); ok {
			return "Toleransi keterlambatan diubah menjadi " + strconv.Itoa(n) + " hari"
		}
		return "Toleransi keterlambatan diubah"
	case "control.restore_all":
		enq, ok1 := mdInt(md, "enqueued")
		skipped, ok2 := mdInt(md, "skipped")
		msg := "Pulihkan Semua dijalankan"
		if ok1 && ok2 {
			msg = fmt.Sprintf("Pulihkan Semua: %d dijadwalkan, %d dilewati", enq, skipped)
		}
		if aborted, _ := mdBool(md, "aborted"); aborted {
			msg += ", dihentikan sebagian"
		}
		return msg
	case "pilot.added":
		return customer(name) + " ditambahkan ke pilot"
	case "pilot.removed":
		return customer(name) + " dihapus dari pilot"
	case "pilot.test_started":
		return "Uji pilot dimulai untuk " + customerLower(name)
	case "pilot.test_stopped":
		return "Uji pilot dihentikan untuk " + customerLower(name)
	case "pilot.test_failed":
		msg := "Uji pilot gagal untuk " + customerLower(name)
		if reason := codeToken(mdString(md, "reason")); reason != "" {
			msg += ": " + reason
		}
		return msg
	case "customer_policy.exempt_set":
		verb := "diubah"
		if exempt, ok := mdBool(md, "exempt"); ok {
			verb = "dicabut"
			if exempt {
				verb = "diberikan"
			}
		}
		msg := "Pengecualian " + verb + " untuk " + customerLower(name)
		if until := mdString(md, "exempted_until"); until != "" {
			if _, err := time.Parse("2006-01-02", until); err == nil {
				msg += " sampai " + until
			}
		}
		return msg
	}
	return "Aktivitas: " + safeText(event, 100)
}

// customerLower is the bare name, or "pelanggan" when unknown.
func customerLower(name string) string {
	if name = safeText(name, 120); name != "" {
		return name
	}
	return "pelanggan"
}
