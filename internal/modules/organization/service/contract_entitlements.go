package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"strconv"
	"strings"
	"time"

	"zyad.cloud/internal/modules/organization/model"
	"zyad.cloud/internal/modules/organization/repository"
)

// ContractEntitlementStore adalah bagian EntitlementRepository yang dipakai penulis entitlement
// contract/default.
type ContractEntitlementStore interface {
	Upsert(context.Context, repository.UpsertEntitlementParams) (model.Entitlement, error)
	ListActiveBySource(ctx context.Context, organizationID string, source model.EntitlementSource, sourceReference *string, at time.Time) ([]model.Entitlement, error)
	HasActiveSource(ctx context.Context, organizationID string, source model.EntitlementSource, at time.Time) (bool, error)
	ExpireBySource(ctx context.Context, organizationID string, source model.EntitlementSource, sourceReference *string, effectiveUntil time.Time, actorUserID, reason string) (int64, error)
	ExpireByID(ctx context.Context, id string, effectiveUntil time.Time, actorUserID, reason string) error
}

// FeatureGrant adalah satu fitur yang diberikan ke workspace. ValueType: boolean|integer|decimal|string.
type FeatureGrant struct {
	Key       string
	ValueType string
	Value     json.RawMessage
}

// ContractEntitlementService menulis dan meng-expire baris organization_entitlements dengan
// source 'contract' dan 'default'. Semua operasinya idempoten: dipanggil ulang dengan masukan yang
// sama (mis. notifikasi DOKU ganda) tidak mengubah baris maupun menaikkan version.
//
// Operasi tidak dibungkus satu transaksi; urutannya dipilih supaya kegagalan di tengah aman
// (pemberian contract ditulis dulu, paket default dicabut terakhir) dan pemanggilan ulang
// menyelesaikannya.
type ContractEntitlementService struct {
	store ContractEntitlementStore
	now   func() time.Time
}

func NewContractEntitlementService(store ContractEntitlementStore) *ContractEntitlementService {
	return &ContractEntitlementService{store: store, now: time.Now}
}

// GrantContract memberikan fitur contract ke workspace, mencabut fitur contract yang sama referensinya
// tetapi sudah tidak ada di daftar, lalu mencabut paket default workspace.
func (s *ContractEntitlementService) GrantContract(
	ctx context.Context,
	organizationID, contractID, contractNumber string,
	grants []FeatureGrant,
	actorUserID string,
) error {
	now := s.now().UTC()
	reason := fmt.Sprintf("contract %s active", contractNumber)
	if err := s.sync(ctx, organizationID, model.EntitlementSourceContract, contractID, grants, actorUserID, reason, now); err != nil {
		return err
	}
	if len(grants) == 0 {
		return nil // contract tanpa fitur tidak boleh menghapus paket gratis
	}
	_, err := s.store.ExpireBySource(ctx, organizationID, model.EntitlementSourceDefault, nil, now, actorUserID, reason)
	return err
}

// RevokeContract mencabut semua fitur contract itu. remainingContract true bila workspace masih punya
// fitur contract aktif lain (pemanggil tidak perlu mengembalikan paket default).
func (s *ContractEntitlementService) RevokeContract(
	ctx context.Context,
	organizationID, contractID, contractNumber string,
	actorUserID string,
) (bool, error) {
	now := s.now().UTC()
	if _, err := s.store.ExpireBySource(ctx, organizationID, model.EntitlementSourceContract, &contractID, now, actorUserID,
		fmt.Sprintf("contract %s ended", contractNumber)); err != nil {
		return false, err
	}
	return s.store.HasActiveSource(ctx, organizationID, model.EntitlementSourceContract, now)
}

// GrantDefault memberikan paket gratis. Tidak melakukan apa pun bila workspace sudah punya contract aktif.
func (s *ContractEntitlementService) GrantDefault(
	ctx context.Context,
	organizationID string,
	grants []FeatureGrant,
	actorUserID string,
) error {
	now := s.now().UTC()
	hasContract, err := s.store.HasActiveSource(ctx, organizationID, model.EntitlementSourceContract, now)
	if err != nil || hasContract {
		return err
	}
	return s.sync(ctx, organizationID, model.EntitlementSourceDefault, "", grants, actorUserID, "default product FREE", now)
}

// sync membuat baris aktif untuk tiap grant (dilewati bila nilainya sudah sama) dan meng-expire baris
// aktif dari sumber+referensi yang sama yang tidak ada lagi di daftar.
func (s *ContractEntitlementService) sync(
	ctx context.Context,
	organizationID string,
	source model.EntitlementSource,
	reference string,
	grants []FeatureGrant,
	actorUserID, reason string,
	now time.Time,
) error {
	var refFilter *string
	if reference != "" {
		refFilter = &reference
	}
	existing, err := s.store.ListActiveBySource(ctx, organizationID, source, refFilter, now)
	if err != nil {
		return err
	}
	byKey := make(map[string]model.Entitlement, len(existing))
	for _, e := range existing {
		byKey[e.FeatureKey] = e
	}

	wanted := make(map[string]bool, len(grants))
	for _, g := range grants {
		limits, err := LimitsFromValue(g.ValueType, g.Value)
		if err != nil {
			return fmt.Errorf("feature %s: %w", g.Key, err)
		}
		wanted[g.Key] = true
		effectiveFrom := now
		if current, ok := byKey[g.Key]; ok {
			if sameLimits(current.Limits, limits) {
				continue
			}
			effectiveFrom = current.EffectiveFrom
		}
		if _, err := s.store.Upsert(ctx, repository.UpsertEntitlementParams{
			OrganizationID:  organizationID,
			FeatureKey:      g.Key,
			Source:          source,
			SourceReference: reference,
			Status:          model.EntitlementStatusActive,
			Limits:          limits,
			EffectiveFrom:   effectiveFrom,
			Reason:          reason,
			ActorUserID:     actorUserID,
		}); err != nil {
			return err
		}
	}
	for _, e := range existing {
		if wanted[e.FeatureKey] {
			continue
		}
		if err := s.store.ExpireByID(ctx, e.ID, now, actorUserID, reason); err != nil {
			return err
		}
	}
	return nil
}

// sameLimits membandingkan limits dari DB (hasil decode JSON) dengan limits baru lewat round-trip JSON
// supaya int64 dan float64 diperlakukan sama.
func sameLimits(current, next map[string]any) bool {
	encoded, err := json.Marshal(next)
	if err != nil {
		return false
	}
	var normalized map[string]any
	if err := json.Unmarshal(encoded, &normalized); err != nil {
		return false
	}
	return reflect.DeepEqual(current, normalized)
}

// LimitsFromValue menerjemahkan nilai fitur ke format limits runtime:
// boolean → {value, enabled}; integer → {value, limit}; decimal/string → {value}.
func LimitsFromValue(valueType string, value json.RawMessage) (map[string]any, error) {
	value = bytes.TrimSpace(value)
	switch valueType {
	case "boolean":
		var b bool
		if err := json.Unmarshal(value, &b); err != nil {
			return nil, fmt.Errorf("boolean value %s: %w", value, err)
		}
		return map[string]any{"value": b, "enabled": b}, nil
	case "integer":
		var n json.Number
		if err := json.Unmarshal(value, &n); err != nil {
			return nil, fmt.Errorf("integer value %s: %w", value, err)
		}
		i, err := strconv.ParseInt(n.String(), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("integer value %s is not a whole number", value)
		}
		return map[string]any{"value": i, "limit": i}, nil
	case "decimal":
		var text string
		if err := json.Unmarshal(value, &text); err != nil {
			var n json.Number
			if err := json.Unmarshal(value, &n); err != nil {
				return nil, fmt.Errorf("decimal value %s is not a number", value)
			}
			text = n.String()
		}
		if _, ok := new(big.Rat).SetString(strings.TrimSpace(text)); !ok {
			return nil, fmt.Errorf("decimal value %s is not a number", value)
		}
		return map[string]any{"value": strings.TrimSpace(text)}, nil
	case "string":
		var text string
		if err := json.Unmarshal(value, &text); err != nil {
			return nil, fmt.Errorf("string value %s: %w", value, err)
		}
		return map[string]any{"value": strings.TrimSpace(text)}, nil
	default:
		return nil, fmt.Errorf("unsupported feature value type %q", valueType)
	}
}
