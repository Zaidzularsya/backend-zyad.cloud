package dto

import (
	"time"

	"zyad.cloud/internal/modules/mikrotik/domain"
	"zyad.cloud/internal/modules/mikrotik/repository"
	"zyad.cloud/internal/modules/mikrotik/service"
)

// SecretListQuery filters the cached PPP secrets. Paired nil means both.
type SecretListQuery struct {
	RouterID string `form:"router_id" binding:"omitempty,uuid"`
	Paired   *bool  `form:"paired"`
	Q        string `form:"q" binding:"max=100"`
	// Suggestion narrows unpaired secrets by auto-match state.
	Suggestion string `form:"suggestion" binding:"omitempty,oneof=suggested needs_decision"`
	// NeedsOriginalProfile nil means both.
	NeedsOriginalProfile *bool `form:"needs_original_profile"`
	Page                 int   `form:"page"`
	PerPage              int   `form:"per_page"`
}

type PairSecretRequest struct {
	AccountID string `json:"account_id" binding:"required,uuid"`
}

type SetOriginalProfileRequest struct {
	Profile string `json:"profile" binding:"required,max=100"`
}

type AccountSearchQuery struct {
	Q string `form:"q" binding:"required,max=100"`
}

// SecretResponse never carries the PPP password or any credential: the cache
// does not store one and this view must not grow one.
type SecretResponse struct {
	ID                   string  `json:"id"`
	RouterID             string  `json:"router_id"`
	Name                 string  `json:"name"`
	Profile              string  `json:"profile"`
	OriginalProfile      string  `json:"original_profile"`
	NeedsOriginalProfile bool    `json:"needs_original_profile"`
	Disabled             bool    `json:"disabled"`
	Present              bool    `json:"present"`
	IsolationState       string  `json:"isolation_state"`
	AccountID            *string `json:"account_id"`
	// AccountName is "" when unpaired or the account was soft-deleted.
	AccountName string     `json:"account_name"`
	PairedAt    *time.Time `json:"paired_at"`
	LastSeenAt  *time.Time `json:"last_seen_at"`
	Comment     string     `json:"comment"`
	// Suggestion is null when there is no hint. For basis "ambiguous" the
	// account_* fields are empty and candidates >= 2.
	Suggestion *SuggestionResponse `json:"suggestion"`
}

type SuggestionResponse struct {
	AccountID    string `json:"account_id"`
	AccountName  string `json:"account_name"`
	AccountPhone string `json:"account_phone"`
	Basis        string `json:"basis"`
	Candidates   int    `json:"candidates"`
}

// MaxPairBulkItems bounds one bulk pair request.
const MaxPairBulkItems = 200

type PairBulkItem struct {
	SecretID  string `json:"secret_id" binding:"required,uuid"`
	AccountID string `json:"account_id" binding:"required,uuid"`
}

type PairBulkRequest struct {
	Items []PairBulkItem `json:"items" binding:"required,min=1,max=200,dive"`
}

type PairBulkResultResponse struct {
	SecretID string `json:"secret_id"`
	OK       bool   `json:"ok"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}

type PairBulkResponse struct {
	Results []PairBulkResultResponse `json:"results"`
}

func PairBulkFromService(results []service.PairResult) PairBulkResponse {
	out := PairBulkResponse{Results: make([]PairBulkResultResponse, 0, len(results))}
	for _, r := range results {
		out.Results = append(out.Results, PairBulkResultResponse{SecretID: r.SecretID, OK: r.OK, Code: r.Code, Message: r.Message})
	}
	return out
}

type AnalyzeResultResponse struct {
	Total         int `json:"total"`
	Suggested     int `json:"suggested"`
	NeedsDecision int `json:"needs_decision"`
}

func AnalyzeResultFromService(r service.AnalyzeResult) AnalyzeResultResponse {
	return AnalyzeResultResponse{Total: r.Total, Suggested: r.Suggested, NeedsDecision: r.NeedsDecision}
}

func SecretFromDomain(s domain.PPPSecret) SecretResponse {
	var sg *SuggestionResponse
	if s.Suggestion != nil {
		sg = &SuggestionResponse{
			AccountID: s.Suggestion.AccountID, AccountName: s.Suggestion.AccountName, AccountPhone: s.Suggestion.AccountPhone,
			Basis: s.Suggestion.Basis, Candidates: s.Suggestion.Candidates,
		}
	}
	return SecretResponse{
		Suggestion: sg,
		ID:         s.ID, RouterID: s.RouterID, Name: s.Name, Profile: s.Profile,
		OriginalProfile: s.OriginalProfile, NeedsOriginalProfile: s.NeedsOriginalProfile,
		Disabled: s.Disabled, Present: s.Present, IsolationState: s.IsolationState,
		AccountID: s.AccountID, AccountName: s.AccountName, PairedAt: s.PairedAt, LastSeenAt: s.LastSeenAt, Comment: s.Comment,
	}
}

func SecretListFromDomain(secrets []domain.PPPSecret) []SecretResponse {
	out := make([]SecretResponse, 0, len(secrets))
	for _, s := range secrets {
		out = append(out, SecretFromDomain(s))
	}
	return out
}

type ImportResultResponse struct {
	Created int `json:"created"`
	Updated int `json:"updated"`
	Missing int `json:"missing"`
	Skipped int `json:"skipped"`
}

func ImportResultFromRepo(r repository.ImportResult) ImportResultResponse {
	return ImportResultResponse{Created: r.Created, Updated: r.Updated, Missing: r.Missing, Skipped: r.Skipped}
}

type AccountResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Phone string `json:"phone"`
	Email string `json:"email"`
}

func AccountListFromService(accounts []service.AccountRef) []AccountResponse {
	out := make([]AccountResponse, 0, len(accounts))
	for _, a := range accounts {
		out = append(out, AccountResponse{ID: a.ID, Name: a.Name, Phone: a.Phone, Email: a.Email})
	}
	return out
}
