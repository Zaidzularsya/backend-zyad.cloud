package dto

import (
	"errors"
	"time"

	"zyad.cloud/internal/modules/mikrotik/service"
)

type ControlStateResponse struct {
	Mode            string     `json:"mode"`
	GraceDays       int        `json:"grace_days"`
	PilotVerifiedAt *time.Time `json:"pilot_verified_at"`
	Engine          string     `json:"engine"`
	EffectiveMode   string     `json:"effective_mode"`
	PilotCount      int        `json:"pilot_count"`
	TestActiveCount int        `json:"test_active_count"`
	IsolatedSecrets int        `json:"isolated_secrets"`
}

func ControlStateFromService(s service.ControlState) ControlStateResponse {
	return ControlStateResponse{
		Mode: s.Mode, GraceDays: s.GraceDays, PilotVerifiedAt: s.PilotVerifiedAt, Engine: s.Engine,
		EffectiveMode: s.EffectiveMode, PilotCount: s.PilotCount, TestActiveCount: s.TestActiveCount,
		IsolatedSecrets: s.IsolatedSecrets,
	}
}

type SetGraceDaysRequest struct {
	GraceDays *int `json:"grace_days" binding:"required"`
}

type ExemptionsRequest struct {
	AccountIDs []string `json:"account_ids"`
	Reason     string   `json:"reason"`
	// Until is the last exempt business day (YYYY-MM-DD); empty means no expiry.
	Until string `json:"until"`
}

type SetModeRequest struct {
	Mode       string             `json:"mode" binding:"required"`
	Exemptions *ExemptionsRequest `json:"exemptions"`
}

// ToInput converts the request; an unparsable until date returns an error.
func (r SetModeRequest) ToInput() (service.SetModeInput, error) {
	in := service.SetModeInput{Mode: r.Mode}
	if r.Exemptions == nil {
		return in, nil
	}
	ex := &service.ExemptionInput{AccountIDs: r.Exemptions.AccountIDs, Reason: r.Exemptions.Reason}
	if r.Exemptions.Until != "" {
		t, err := time.Parse(dateLayout, r.Exemptions.Until)
		if err != nil {
			return service.SetModeInput{}, errors.New("exemptions.until must be a date in YYYY-MM-DD format")
		}
		ex.Until = &t
	}
	in.Exemptions = ex
	return in, nil
}

type PilotAccountResponse struct {
	AccountID   string     `json:"account_id"`
	AccountName string     `json:"account_name"`
	AddedAt     time.Time  `json:"added_at"`
	TestActive  bool       `json:"test_active"`
	TestUntil   *time.Time `json:"test_until"`
	Verified    bool       `json:"verified"`
}

func PilotListFromService(items []service.PilotView) []PilotAccountResponse {
	out := make([]PilotAccountResponse, 0, len(items))
	for _, p := range items {
		out = append(out, PilotAccountResponse{
			AccountID: p.AccountID, AccountName: p.AccountName, AddedAt: p.AddedAt,
			TestActive: p.TestActive, TestUntil: p.TestUntil, Verified: p.Verified,
		})
	}
	return out
}

type PilotTestResponse struct {
	TestUntil time.Time `json:"test_until"`
}

type RestoreAllResponse struct {
	Enqueued int `json:"enqueued"`
	Skipped  int `json:"skipped"`
	Failed   int `json:"failed"`
}

func RestoreAllFromService(r service.RestoreAllResult) RestoreAllResponse {
	return RestoreAllResponse{Enqueued: r.Enqueued, Skipped: r.Skipped, Failed: r.Failed}
}
