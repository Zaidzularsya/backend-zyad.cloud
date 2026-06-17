package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/platform/database"
)

type analyticsRepository struct {
	db *database.Pool
}

func NewAnalyticsRepository(db *database.Pool) AnalyticsRepository {
	return &analyticsRepository{db: db}
}

func (r *analyticsRepository) withTx(ctx context.Context, scope coretenant.Scope, fn func(pgx.Tx) error) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, "SELECT set_config('app.organization_id', $1, true)", scope.OrganizationID())
	if err != nil {
		return err
	}

	if err := fn(tx); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *analyticsRepository) RecordEvent(ctx context.Context, scope coretenant.Scope, params CreateAnalyticsEventParams) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	query := `
		INSERT INTO landing_analytics_events (
			organization_id, landing_page_id, version, event_name,
			section_key, target_key, session_id,
			utm_source, utm_medium, utm_campaign, utm_term, utm_content,
			referrer, browser, device, os, ip_address_hash
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
			$11, $12, $13, $14, $15, $16, $17
		)
	`

	return r.withTx(ctx, scope, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, query,
			scope.OrganizationID(),
			params.LandingPageID,
			params.Version,
			params.EventName,
			params.SectionKey,
			params.TargetKey,
			params.SessionID,
			params.UTMSource,
			params.UTMMedium,
			params.UTMCampaign,
			params.UTMTerm,
			params.UTMContent,
			params.Referrer,
			params.Browser,
			params.Device,
			params.OS,
			params.IPAddressHash,
		)
		return err
	})
}

func (r *analyticsRepository) ListEvents(ctx context.Context, scope coretenant.Scope, filter AnalyticsFilter) ([]domain.LandingAnalyticsEvent, error) {
	if !scope.IsValid() {
		return nil, coretenant.ErrInvalidScope
	}

	query := `
		SELECT
			id, landing_page_id, version, event_name,
			section_key, target_key, session_id,
			utm_source, utm_medium, utm_campaign, utm_term, utm_content,
			referrer, browser, device, os, ip_address_hash, created_at
		FROM landing_analytics_events
		WHERE organization_id = $1
	`
	args := []interface{}{scope.OrganizationID()}
	argCount := 2

	if filter.LandingPageID != "" {
		query += fmt.Sprintf(" AND landing_page_id = $%d", argCount)
		args = append(args, filter.LandingPageID)
		argCount++
	}
	if filter.EventName != "" {
		query += fmt.Sprintf(" AND event_name = $%d", argCount)
		args = append(args, filter.EventName)
		argCount++
	}
	if filter.StartDate != "" {
		query += fmt.Sprintf(" AND created_at >= $%d", argCount)
		args = append(args, filter.StartDate)
		argCount++
	}
	if filter.EndDate != "" {
		query += fmt.Sprintf(" AND created_at <= $%d", argCount)
		args = append(args, filter.EndDate)
		argCount++
	}

	query += ` ORDER BY created_at DESC`

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argCount)
		args = append(args, filter.Limit)
		argCount++
	}
	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argCount)
		args = append(args, filter.Offset)
		argCount++
	}

	var events []domain.LandingAnalyticsEvent

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var event domain.LandingAnalyticsEvent
			var secKey, tgtKey, sessID, uSrc, uMed, uCam, uTerm, uCon, ref, brow, dev, os, ip *string
			err := rows.Scan(
				&event.ID, &event.LandingPageID, &event.Version, &event.EventName,
				&secKey, &tgtKey, &sessID, &uSrc, &uMed, &uCam, &uTerm, &uCon,
				&ref, &brow, &dev, &os, &ip, &event.CreatedAt,
			)
			if err != nil {
				return err
			}
			event.OrganizationID = scope.OrganizationID()
			if secKey != nil { event.SectionKey = secKey }
			if tgtKey != nil { event.TargetKey = tgtKey }
			if sessID != nil { event.SessionID = sessID }
			if uSrc != nil { event.UTMSource = uSrc }
			if uMed != nil { event.UTMMedium = uMed }
			if uCam != nil { event.UTMCampaign = uCam }
			if uTerm != nil { event.UTMTerm = uTerm }
			if uCon != nil { event.UTMContent = uCon }
			if ref != nil { event.Referrer = ref }
			if brow != nil { event.Browser = brow }
			if dev != nil { event.Device = dev }
			if os != nil { event.OS = os }
			if ip != nil { event.IPAddressHash = ip }

			events = append(events, event)
		}
		return rows.Err()
	})

	if err != nil {
		return nil, err
	}

	return events, nil
}

func (r *analyticsRepository) GetEventCount(ctx context.Context, scope coretenant.Scope, filter AnalyticsFilter) (int64, error) {
	if !scope.IsValid() {
		return 0, coretenant.ErrInvalidScope
	}

	query := `SELECT COUNT(*) FROM landing_analytics_events WHERE organization_id = $1`
	args := []interface{}{scope.OrganizationID()}
	argCount := 2

	if filter.LandingPageID != "" {
		query += fmt.Sprintf(" AND landing_page_id = $%d", argCount)
		args = append(args, filter.LandingPageID)
		argCount++
	}
	if filter.EventName != "" {
		query += fmt.Sprintf(" AND event_name = $%d", argCount)
		args = append(args, filter.EventName)
		argCount++
	}
	if filter.StartDate != "" {
		query += fmt.Sprintf(" AND created_at >= $%d", argCount)
		args = append(args, filter.StartDate)
		argCount++
	}
	if filter.EndDate != "" {
		query += fmt.Sprintf(" AND created_at <= $%d", argCount)
		args = append(args, filter.EndDate)
		argCount++
	}

	var count int64
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, args...).Scan(&count)
	})

	return count, err
}

func (r *analyticsRepository) UpsertDailyStats(ctx context.Context, scope coretenant.Scope, params RecordDailyStatsParams) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	query := `
		INSERT INTO landing_analytics_daily (
			organization_id, landing_page_id, date,
			page_views, unique_visitors, cta_clicks, form_starts, submissions
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8
		)
		ON CONFLICT (organization_id, landing_page_id, date) DO UPDATE SET
			page_views = landing_analytics_daily.page_views + EXCLUDED.page_views,
			unique_visitors = landing_analytics_daily.unique_visitors + EXCLUDED.unique_visitors,
			cta_clicks = landing_analytics_daily.cta_clicks + EXCLUDED.cta_clicks,
			form_starts = landing_analytics_daily.form_starts + EXCLUDED.form_starts,
			submissions = landing_analytics_daily.submissions + EXCLUDED.submissions,
			updated_at = NOW()
	`

	return r.withTx(ctx, scope, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, query,
			scope.OrganizationID(),
			params.LandingPageID,
			params.Date,
			params.PageViews,
			params.UniqueVisitors,
			params.CTAClicks,
			params.FormStarts,
			params.Submissions,
		)
		return err
	})
}

func (r *analyticsRepository) ListDailyStats(ctx context.Context, scope coretenant.Scope, pageID string, startDate string, endDate string) ([]domain.LandingAnalyticsDaily, error) {
	if !scope.IsValid() {
		return nil, coretenant.ErrInvalidScope
	}

	query := `
		SELECT
			id, landing_page_id, date,
			page_views, unique_visitors, cta_clicks, form_starts, submissions,
			created_at, updated_at
		FROM landing_analytics_daily
		WHERE organization_id = $1 AND landing_page_id = $2
	`
	args := []interface{}{scope.OrganizationID(), pageID}
	argCount := 3

	if startDate != "" {
		query += fmt.Sprintf(" AND date >= $%d", argCount)
		args = append(args, startDate)
		argCount++
	}
	if endDate != "" {
		query += fmt.Sprintf(" AND date <= $%d", argCount)
		args = append(args, endDate)
		argCount++
	}

	query += ` ORDER BY date ASC`

	var stats []domain.LandingAnalyticsDaily

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var s domain.LandingAnalyticsDaily
			err := rows.Scan(
				&s.ID, &s.LandingPageID, &s.Date,
				&s.PageViews, &s.UniqueVisitors, &s.CTAClicks, &s.FormStarts, &s.Submissions,
				&s.CreatedAt, &s.UpdatedAt,
			)
			if err != nil {
				return err
			}
			s.OrganizationID = scope.OrganizationID()
			stats = append(stats, s)
		}
		return rows.Err()
	})

	if err != nil {
		return nil, err
	}

	return stats, nil
}
