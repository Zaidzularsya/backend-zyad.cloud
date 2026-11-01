package service

import (
	"context"
	"log/slog"
	"strings"
	"unicode"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/domain"
	"zyad.cloud/internal/modules/mikrotik/repository"
)

const (
	suggestAccountPage = 500
	suggestSecretPage  = 200
	minPhoneDigits     = 8
)

// NormalizePhone keeps digits only and rewrites a leading 0 or 62 to 62.
// It returns "" when fewer than 8 digits remain, so junk never matches.
func NormalizePhone(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	d := b.String()
	if len(d) < minPhoneDigits {
		return ""
	}
	switch {
	case strings.HasPrefix(d, "62"):
	case strings.HasPrefix(d, "0"):
		d = "62" + d[1:]
	}
	if len(d) < minPhoneDigits {
		return ""
	}
	return d
}

func normalizeName(s string) string {
	return strings.ToLower(strings.TrimFunc(s, unicode.IsSpace))
}

// MatchSecret finds the account for a secret name: phone first, then exact
// (case-insensitive, trimmed) name. More than one distinct account at the
// winning level gives basis "ambiguous", accountID "" and the candidate count.
// No match returns "", "", 0. Empty values never match.
func MatchSecret(secretName string, accounts []AccountRef) (accountID, basis string, candidates int) {
	return newAccountIndex(accounts).match(secretName)
}

type accountIndex struct {
	byPhone map[string][]string
	byName  map[string][]string
}

func newAccountIndex(accounts []AccountRef) *accountIndex {
	ix := &accountIndex{byPhone: map[string][]string{}, byName: map[string][]string{}}
	for _, a := range accounts {
		ix.add(a)
	}
	return ix
}

func (ix *accountIndex) add(a AccountRef) {
	if p := NormalizePhone(a.Phone); p != "" {
		ix.byPhone[p] = appendUnique(ix.byPhone[p], a.ID)
	}
	if n := normalizeName(a.Name); n != "" {
		ix.byName[n] = appendUnique(ix.byName[n], a.ID)
	}
}

func appendUnique(ids []string, id string) []string {
	for _, x := range ids {
		if x == id {
			return ids
		}
	}
	return append(ids, id)
}

func (ix *accountIndex) match(secretName string) (string, string, int) {
	if p := NormalizePhone(secretName); p != "" {
		if id, basis, n := pick(ix.byPhone[p], repository.SuggestionBasisPhone); n > 0 {
			return id, basis, n
		}
	}
	if n := normalizeName(secretName); n != "" {
		if id, basis, c := pick(ix.byName[n], repository.SuggestionBasisName); c > 0 {
			return id, basis, c
		}
	}
	return "", "", 0
}

func pick(ids []string, basis string) (string, string, int) {
	switch len(ids) {
	case 0:
		return "", "", 0
	case 1:
		return ids[0], basis, 1
	}
	return "", repository.SuggestionBasisAmbiguous, len(ids)
}

// AnalyzeResult: Total = unpaired present secrets analyzed, Suggested = rows
// with a single phone/name match, NeedsDecision = ambiguous rows + secrets
// without any match.
type AnalyzeResult struct{ Total, Suggested, NeedsDecision int }

// SecretLister is the read side of the secret repository the analysis needs.
type SecretLister interface {
	List(ctx context.Context, scope coretenant.Scope, f repository.SecretFilter, limit, offset int) ([]domain.PPPSecret, int64, error)
}

type SuggestionService struct {
	routers  RouterReader
	secrets  SecretLister
	accounts AccountDirectory
	store    repository.SuggestionRepository
	locker   RouterLocker
}

func NewSuggestionService(routers RouterReader, secrets SecretLister, accounts AccountDirectory, store repository.SuggestionRepository, locker RouterLocker) *SuggestionService {
	return &SuggestionService{routers: routers, secrets: secrets, accounts: accounts, store: store, locker: locker}
}

// Analyze recomputes the pairing suggestions of one router. Only present,
// unpaired secrets are considered; the router's previous suggestions are
// replaced atomically. mikrotik.ErrAnalyzeInProgress when another analysis of
// the same router is running.
func (s *SuggestionService) Analyze(ctx context.Context, scope coretenant.Scope, routerID, actor string) (AnalyzeResult, error) {
	if !validID(routerID) {
		return AnalyzeResult{}, mikrotik.ErrRouterNotFound
	}
	if _, err := s.routers.Get(ctx, scope, routerID); err != nil {
		return AnalyzeResult{}, err
	}
	unlock, ok, err := s.locker.TryLock(ctx, "mikrotik.suggest:"+scope.OrganizationID()+":"+routerID)
	if err != nil {
		slog.Warn("mikrotik: acquire suggestion lock failed", "router_id", routerID, "error", scrub(err.Error()))
		return AnalyzeResult{}, err
	}
	if !ok {
		return AnalyzeResult{}, mikrotik.ErrAnalyzeInProgress
	}
	defer unlock()

	secrets, err := s.unpairedSecrets(ctx, scope, routerID)
	if err != nil {
		return AnalyzeResult{}, err
	}
	ix := newAccountIndex(nil)
	for after := ""; ; {
		page, err := s.accounts.List(ctx, scope, after, suggestAccountPage)
		if err != nil {
			return AnalyzeResult{}, err
		}
		for _, a := range page {
			ix.add(a)
		}
		// An adapter may cap the page below the requested size: only an empty page ends the walk.
		if len(page) == 0 {
			break
		}
		after = page[len(page)-1].ID
	}

	res := AnalyzeResult{Total: len(secrets)}
	rows := make([]repository.Suggestion, 0, len(secrets))
	for _, sec := range secrets {
		id, basis, n := ix.match(sec.Name)
		switch basis {
		case "":
			res.NeedsDecision++
			continue
		case repository.SuggestionBasisAmbiguous:
			res.NeedsDecision++
		default:
			res.Suggested++
		}
		rows = append(rows, repository.Suggestion{SecretID: sec.ID, AccountID: id, Basis: basis, Candidates: n})
	}
	if err := s.store.ReplaceForRouter(ctx, scope, routerID, rows); err != nil {
		return AnalyzeResult{}, err
	}
	slog.Info("mikrotik: suggestions computed", "router_id", routerID, "actor", actor, "total", res.Total, "suggested", res.Suggested)
	return res, nil
}

func (s *SuggestionService) unpairedSecrets(ctx context.Context, scope coretenant.Scope, routerID string) ([]domain.PPPSecret, error) {
	unpaired := false
	f := repository.SecretFilter{RouterID: routerID, Paired: &unpaired}
	var out []domain.PPPSecret
	for offset := 0; ; offset += suggestSecretPage {
		page, total, err := s.secrets.List(ctx, scope, f, suggestSecretPage, offset)
		if err != nil {
			return nil, err
		}
		for _, sec := range page {
			if sec.Present && sec.AccountID == nil {
				out = append(out, sec)
			}
		}
		if len(page) == 0 || int64(offset+len(page)) >= total {
			return out, nil
		}
	}
}
