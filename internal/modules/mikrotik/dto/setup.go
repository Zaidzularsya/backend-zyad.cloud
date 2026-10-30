package dto

import "zyad.cloud/internal/modules/mikrotik/domain"

// SetupRequest carries the operator-chosen setup placeholders (see
// docs/runbook-mikrotik-isolir-setup.md). Empty redirect_url means no redirect
// objects are planned.
type SetupRequest struct {
	ProfileName      string   `json:"profile_name" binding:"max=63"`
	PoolCIDR         string   `json:"pool_cidr" binding:"max=64"`
	RateLimit        string   `json:"rate_limit" binding:"max=32"`
	PlatformHost     string   `json:"platform_host" binding:"max=253"`
	TenantIsolirHost string   `json:"tenant_isolir_host" binding:"max=253"`
	DokuHosts        []string `json:"doku_hosts" binding:"max=20,dive,max=253"`
	RedirectURL      string   `json:"redirect_url" binding:"max=255"`
}

// ApplySetupRequest is the setup input plus the hash returned by preview.
type ApplySetupRequest struct {
	SetupRequest
	PlanHash string `json:"plan_hash" binding:"required,len=64"`
}

type SetupActionResponse struct {
	Kind     string            `json:"kind"`
	Path     string            `json:"path"`
	Match    map[string]string `json:"match"`
	Attrs    map[string]string `json:"attrs"`
	Optional bool              `json:"optional,omitempty"`
	Note     string            `json:"note,omitempty"`
}

type SetupPlanResponse struct {
	Hash    string                `json:"hash"`
	Actions []SetupActionResponse `json:"actions"`
}

func SetupPlanFromDomain(p domain.SetupPlan) SetupPlanResponse {
	out := SetupPlanResponse{Hash: p.Hash, Actions: make([]SetupActionResponse, 0, len(p.Actions))}
	for _, a := range p.Actions {
		out.Actions = append(out.Actions, SetupActionResponse{
			Kind: a.Kind, Path: a.Path, Match: a.Match, Attrs: a.Attrs, Optional: a.Optional, Note: a.Note,
		})
	}
	return out
}
