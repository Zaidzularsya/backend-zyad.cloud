package model

import "time"

type FeatureValueType string

const (
	FeatureValueTypeBoolean FeatureValueType = "boolean"
	FeatureValueTypeInteger FeatureValueType = "integer"
	FeatureValueTypeString  FeatureValueType = "string"
	FeatureValueTypeDecimal FeatureValueType = "decimal"
)

func (t FeatureValueType) IsValid() bool {
	switch t {
	case FeatureValueTypeBoolean,
		FeatureValueTypeInteger,
		FeatureValueTypeString,
		FeatureValueTypeDecimal:
		return true
	default:
		return false
	}
}

type ResetStrategy string

const (
	ResetStrategyNever   ResetStrategy = "never"
	ResetStrategyMonthly ResetStrategy = "monthly"
	ResetStrategyYearly  ResetStrategy = "yearly"
	ResetStrategyCustom  ResetStrategy = "custom"
)

func (s ResetStrategy) IsValid() bool {
	switch s {
	case ResetStrategyNever,
		ResetStrategyMonthly,
		ResetStrategyYearly,
		ResetStrategyCustom:
		return true
	default:
		return false
	}
}

type Feature struct {
	ID            string
	Key           string
	Module        string
	Name          string
	Description   string
	ValueType     FeatureValueType
	Unit          string
	ResetStrategy ResetStrategy
	IsActive      bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type PlanEntitlement struct {
	ID           string
	PlanID       string
	FeatureID    string
	FeatureKey   string
	ValueBool    *bool
	ValueInt     *int64
	ValueDecimal *string
	ValueString  *string
	Limits       map[string]any
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (e PlanEntitlement) HasSingleValue() bool {
	count := 0
	if e.ValueBool != nil {
		count++
	}
	if e.ValueInt != nil {
		count++
	}
	if e.ValueDecimal != nil {
		count++
	}
	if e.ValueString != nil {
		count++
	}
	return count == 1
}
