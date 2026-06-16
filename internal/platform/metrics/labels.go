package metrics

import (
	"errors"
	"regexp"
	"sort"
	"strings"

	coretenant "zyad.cloud/internal/core/tenant"
)

var (
	ErrInvalidLabel    = errors.New("invalid metric label")
	ErrHighCardinality = errors.New("metric label has unbounded cardinality")

	labelNameExpression  = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
	labelValueExpression = regexp.MustCompile(`^[a-z0-9_.-]{0,64}$`)
)

var highCardinalityLabels = map[string]struct{}{
	"actor_user_id":            {},
	"email":                    {},
	"effective_user_id":        {},
	"impersonation_session_id": {},
	"ip_address":               {},
	"membership_id":            {},
	"organization_id":          {},
	"request_id":               {},
	"session_id":               {},
	"tenant_id":                {},
	"user_id":                  {},
}

type LabelSet map[string]string

func NewLabelSet(labels map[string]string) (LabelSet, error) {
	labelSet := make(LabelSet, len(labels))
	for key, value := range labels {
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.ToLower(strings.TrimSpace(value))
		if !labelNameExpression.MatchString(key) ||
			!labelValueExpression.MatchString(value) {
			return nil, ErrInvalidLabel
		}
		if _, blocked := highCardinalityLabels[key]; blocked {
			return nil, ErrHighCardinality
		}
		labelSet[key] = value
	}
	return labelSet, nil
}

func OrganizationHealthLabels(
	organizationType coretenant.OrganizationType,
	organizationStatus coretenant.OrganizationStatus,
	dataPlacement coretenant.DataPlacement,
	healthStatus string,
) (LabelSet, error) {
	return NewLabelSet(map[string]string{
		"organization_type":   string(organizationType),
		"organization_status": string(organizationStatus),
		"data_placement":      string(dataPlacement),
		"health_status":       healthStatus,
	})
}

func ResolutionLabels(source coretenant.ResolutionSource, result string) (LabelSet, error) {
	return NewLabelSet(map[string]string{
		"resolution_source": string(source),
		"result":            result,
	})
}

func (s LabelSet) SortedAttrs() []string {
	attrs := make([]string, 0, len(s))
	for key, value := range s {
		attrs = append(attrs, key+"="+value)
	}
	sort.Strings(attrs)
	return attrs
}
