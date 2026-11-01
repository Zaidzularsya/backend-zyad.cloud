package dto

import "zyad.cloud/internal/modules/mikrotik/service"

type OnboardingResponse struct {
	RouterRegistered    bool `json:"router_registered"`
	RouterReady         bool `json:"router_ready"`
	SecretsTotal        int  `json:"secrets_total"`
	SecretsPaired       int  `json:"secrets_paired"`
	SecretsNeedsProfile int  `json:"secrets_needs_profile"`
	ProductsMarked      int  `json:"products_marked"`
	SenderConfigured    bool `json:"sender_configured"`
	Complete            bool `json:"complete"`
}

func OnboardingFromService(s service.OnboardingSummary) OnboardingResponse {
	return OnboardingResponse{
		RouterRegistered: s.RouterRegistered, RouterReady: s.RouterReady,
		SecretsTotal: s.SecretsTotal, SecretsPaired: s.SecretsPaired, SecretsNeedsProfile: s.SecretsNeedsProfile,
		ProductsMarked: s.ProductsMarked, SenderConfigured: s.SenderConfigured, Complete: s.Complete,
	}
}

type CustomerStatusResponse struct {
	AccountID      string `json:"account_id"`
	IsolationState string `json:"isolation_state"`
	Exempt         bool   `json:"exempt"`
	Secrets        int    `json:"secrets"`
}

func CustomerStatusesFromService(items []service.CustomerStatus) []CustomerStatusResponse {
	out := make([]CustomerStatusResponse, 0, len(items))
	for _, it := range items {
		out = append(out, CustomerStatusResponse{AccountID: it.AccountID, IsolationState: it.IsolationState, Exempt: it.Exempt, Secrets: it.Secrets})
	}
	return out
}
