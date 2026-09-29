package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/core/businesstime"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/platform/database"
)

type leadDashboardRepository struct {
	db  *database.Pool
	now func() time.Time
}

func NewLeadDashboardRepository(db *database.Pool) LeadDashboardRepository {
	return NewLeadDashboardRepositoryWithClock(db, time.Now)
}

// NewLeadDashboardRepositoryWithClock lets tests pin "now" for the
// follow-up windows, which are relative to the current business day.
func NewLeadDashboardRepositoryWithClock(db *database.Pool, now func() time.Time) LeadDashboardRepository {
	return &leadDashboardRepository{db: db, now: now}
}

// Timestamps in crm_* are "timestamp without time zone" holding UTC
// wall-clock (the pool pins the session timezone to UTC). Dashboard days are
// business days (Asia/Jakarta): Go converts every day boundary to a UTC
// instant, and daily/monthly buckets shift by the business UTC offset, so
// nothing here depends on the database session timezone.
//
// Names of users (owner, assignee, actor) are resolved only through
// organization_memberships of the same organization — same rule as
// crm_leads.owner_name — so a foreign user id cannot leak another tenant's
// user name.
const memberNameSQL = `(
	SELECT u.name FROM users u
	JOIN organization_memberships m ON m.user_id = u.id AND m.organization_id = $1
	WHERE u.id = %s
)`

func memberName(column string) string {
	return fmt.Sprintf(memberNameSQL, column)
}

// withTx runs read queries in a transaction with app.organization_id set,
// required by the RLS policies on crm_* tables.
func (r *leadDashboardRepository) withTx(ctx context.Context, scope coretenant.Scope, fn func(pgx.Tx) error) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, "SELECT set_config('app.organization_id', $1, true)", scope.OrganizationID()); err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *leadDashboardRepository) Dashboard(ctx context.Context, scope coretenant.Scope, rng domain.LeadDashboardRange, limits LeadDashboardLimits) (domain.LeadDashboard, error) {
	if !scope.IsValid() {
		return domain.LeadDashboard{}, coretenant.ErrInvalidScope
	}

	orgID := scope.OrganizationID()
	// Exclusive upper bounds: the day after To. rng holds calendar dates; the
	// queries filter on the UTC instants where those business days start.
	calFrom, calToExcl := rng.From, rng.To.AddDate(0, 0, 1)
	from, toExcl := businesstime.DayStartUTC(calFrom), businesstime.DayStartUTC(calToExcl)
	prevFrom, prevToExcl := businesstime.DayStartUTC(rng.PreviousFrom), businesstime.DayStartUTC(rng.PreviousTo.AddDate(0, 0, 1))
	// Business UTC offset used to bucket stored UTC timestamps into local days.
	_, offsetSecs := from.In(businesstime.Location()).Zone()

	// Follow-up windows relative to "today" in business time.
	today := businesstime.DayOf(r.now())
	todayStart := businesstime.DayStartUTC(today)
	tomorrowStart := businesstime.DayStartUTC(today.AddDate(0, 0, 1))
	weekEnd := businesstime.DayStartUTC(today.AddDate(0, 0, 8))

	d := domain.LeadDashboard{
		StatusCounts:          map[domain.LeadStatus]int64{},
		StatusEntered:         map[domain.LeadStatus]int64{},
		PreviousStatusEntered: map[domain.LeadStatus]int64{},
		Series:                []domain.LeadDashboardBucket{},
		BySource:              []domain.LeadSourceCount{},
		UpcomingFollowUps:     []domain.LeadFollowUp{},
		RecentActivity:        []domain.LeadActivityItem{},
	}

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		// 1. Current status distribution.
		rows, err := tx.Query(ctx, `
			SELECT status, COUNT(*) FROM crm_leads
			WHERE organization_id = $1 AND deleted_at IS NULL
			GROUP BY status`, orgID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var status string
			var n int64
			if err := rows.Scan(&status, &n); err != nil {
				rows.Close()
				return err
			}
			d.StatusCounts[domain.LeadStatus(status)] = n
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		// 2. Created / converted in the range and the previous range.
		err = tx.QueryRow(ctx, `
			SELECT
				COUNT(*) FILTER (WHERE created_at >= $2 AND created_at < $3),
				COUNT(*) FILTER (WHERE converted_at >= $2 AND converted_at < $3),
				COUNT(*) FILTER (WHERE created_at >= $4 AND created_at < $5),
				COUNT(*) FILTER (WHERE converted_at >= $4 AND converted_at < $5)
			FROM crm_leads
			WHERE organization_id = $1 AND deleted_at IS NULL
				AND (created_at >= $4 OR converted_at >= $4)`,
			orgID, from, toExcl, prevFrom, prevToExcl,
		).Scan(&d.Created, &d.Converted, &d.PreviousCreated, &d.PreviousConverted)
		if err != nil {
			return err
		}

		// 3. Leads entering each status (event history), both ranges.
		rows, err = tx.Query(ctx, `
			SELECT e.to_value,
				COUNT(*) FILTER (WHERE e.created_at >= $2 AND e.created_at < $3),
				COUNT(*) FILTER (WHERE e.created_at >= $4 AND e.created_at < $5)
			FROM crm_lead_events e
			JOIN crm_leads l ON l.organization_id = e.organization_id AND l.id = e.lead_id AND l.deleted_at IS NULL
			WHERE e.organization_id = $1
				AND e.event_type IN ('created', 'status_changed', 'converted')
				AND e.to_value IS NOT NULL
				AND e.created_at >= $4 AND e.created_at < $3
			GROUP BY e.to_value`,
			orgID, from, toExcl, prevFrom, prevToExcl)
		if err != nil {
			return err
		}
		for rows.Next() {
			var status string
			var cur, prev int64
			if err := rows.Scan(&status, &cur, &prev); err != nil {
				rows.Close()
				return err
			}
			d.StatusEntered[domain.LeadStatus(status)] = cur
			d.PreviousStatusEntered[domain.LeadStatus(status)] = prev
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		// 4. Time series, one row per bucket including empty ones.
		unit := string(rng.Granularity)
		rows, err = tx.Query(ctx, `
			WITH buckets AS (
				SELECT generate_series(date_trunc($4, $6::timestamp), date_trunc($4, $7::timestamp - interval '1 day'), ('1 ' || $4)::interval) AS bucket
			),
			created AS (
				SELECT date_trunc($4, created_at + make_interval(secs => $5)) AS bucket, COUNT(*) AS n FROM crm_leads
				WHERE organization_id = $1 AND deleted_at IS NULL AND created_at >= $2 AND created_at < $3
				GROUP BY 1
			),
			converted AS (
				SELECT date_trunc($4, converted_at + make_interval(secs => $5)) AS bucket, COUNT(*) AS n FROM crm_leads
				WHERE organization_id = $1 AND deleted_at IS NULL AND converted_at >= $2 AND converted_at < $3
				GROUP BY 1
			)
			SELECT b.bucket, COALESCE(c.n, 0), COALESCE(v.n, 0)
			FROM buckets b
			LEFT JOIN created c ON c.bucket = b.bucket
			LEFT JOIN converted v ON v.bucket = b.bucket
			ORDER BY b.bucket`,
			orgID, from, toExcl, unit, float64(offsetSecs), calFrom, calToExcl)
		if err != nil {
			return err
		}
		for rows.Next() {
			var b domain.LeadDashboardBucket
			if err := rows.Scan(&b.Bucket, &b.Created, &b.Converted); err != nil {
				rows.Close()
				return err
			}
			d.Series = append(d.Series, b)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		// 5. Source breakdown of leads created in the range.
		rows, err = tx.Query(ctx, `
			SELECT COALESCE(NULLIF(btrim(lower(source)), ''), '') AS src, COUNT(*)
			FROM crm_leads
			WHERE organization_id = $1 AND deleted_at IS NULL AND created_at >= $2 AND created_at < $3
			GROUP BY 1
			ORDER BY 2 DESC, 1`,
			orgID, from, toExcl)
		if err != nil {
			return err
		}
		for rows.Next() {
			var s domain.LeadSourceCount
			if err := rows.Scan(&s.Source, &s.Count); err != nil {
				rows.Close()
				return err
			}
			d.BySource = append(d.BySource, s)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		// 6. Follow-up summary: pending activities on live leads, relative
		// to "today" in business time.
		err = tx.QueryRow(ctx, `
			SELECT
				COUNT(*),
				COUNT(*) FILTER (WHERE a.due_at < $2),
				COUNT(*) FILTER (WHERE a.due_at >= $2 AND a.due_at < $3),
				COUNT(*) FILTER (WHERE a.due_at >= $3 AND a.due_at < $4)
			FROM crm_activities a
			JOIN crm_leads l ON l.organization_id = a.organization_id AND l.id = a.related_entity_id AND l.deleted_at IS NULL
			WHERE a.organization_id = $1 AND a.related_entity_type = 'lead'
				AND a.status = 'pending' AND a.deleted_at IS NULL`,
			orgID,
			todayStart, tomorrowStart, weekEnd,
		).Scan(&d.FollowUps.Pending, &d.FollowUps.Overdue, &d.FollowUps.DueToday, &d.FollowUps.DueNext7Days)
		if err != nil {
			return err
		}

		// 7. Upcoming follow-ups: overdue first, undated last.
		rows, err = tx.Query(ctx, `
			SELECT `+prefixedActivityColumns("a")+`,
				l.contact_name, COALESCE(l.company_name, ''),
				COALESCE(`+memberName("a.assignee_user_id")+`, '')
			FROM crm_activities a
			JOIN crm_leads l ON l.organization_id = a.organization_id AND l.id = a.related_entity_id AND l.deleted_at IS NULL
			WHERE a.organization_id = $1 AND a.related_entity_type = 'lead'
				AND a.status = 'pending' AND a.deleted_at IS NULL
			ORDER BY a.due_at NULLS LAST, a.created_at
			LIMIT $2`,
			orgID, limits.FollowUps)
		if err != nil {
			return err
		}
		for rows.Next() {
			var f domain.LeadFollowUp
			a, err := scanActivityWith(rows, &f.LeadName, &f.CompanyName, &f.AssigneeName)
			if err != nil {
				rows.Close()
				return err
			}
			f.Activity = a
			d.UpcomingFollowUps = append(d.UpcomingFollowUps, f)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		// 8. Recent activity: lead events merged with activities on leads.
		rows, err = tx.Query(ctx, `
			SELECT kind, occurred_at, lead_id, lead_name, COALESCE(actor_id::text, ''),
				COALESCE(`+memberName("feed.actor_id")+`, ''),
				from_value, to_value,
				CASE WHEN kind = 'assigned' AND from_value <> '' THEN COALESCE(`+memberName("from_value::uuid")+`, '') ELSE '' END,
				CASE WHEN kind = 'assigned' AND to_value <> '' THEN COALESCE(`+memberName("to_value::uuid")+`, '') ELSE '' END,
				activity_id, activity_type, subject
			FROM (
				(SELECT e.event_type AS kind, e.created_at AS occurred_at, e.lead_id, l.contact_name AS lead_name,
					e.actor_user_id AS actor_id,
					COALESCE(e.from_value, '') AS from_value, COALESCE(e.to_value, '') AS to_value,
					'' AS activity_id, '' AS activity_type, '' AS subject
				FROM crm_lead_events e
				JOIN crm_leads l ON l.organization_id = e.organization_id AND l.id = e.lead_id
				WHERE e.organization_id = $1
					AND (l.deleted_at IS NULL OR e.event_type = 'deleted')
				ORDER BY e.created_at DESC
				LIMIT $2)
				UNION ALL
				(SELECT
					CASE WHEN a.status = 'completed' THEN 'activity_completed' ELSE 'activity_created' END,
					COALESCE(a.completed_at, a.created_at), a.related_entity_id, l.contact_name,
					CASE WHEN a.status = 'completed' THEN COALESCE(a.updated_by, a.created_by) ELSE a.created_by END,
					'', '', a.id::text, a.type, a.subject
				FROM crm_activities a
				JOIN crm_leads l ON l.organization_id = a.organization_id AND l.id = a.related_entity_id AND l.deleted_at IS NULL
				WHERE a.organization_id = $1 AND a.related_entity_type = 'lead'
					AND a.deleted_at IS NULL AND a.status <> 'cancelled'
				ORDER BY COALESCE(a.completed_at, a.created_at) DESC
				LIMIT $2)
			) feed
			ORDER BY occurred_at DESC
			LIMIT $2`,
			orgID, limits.RecentActivity)
		if err != nil {
			return err
		}
		for rows.Next() {
			var it domain.LeadActivityItem
			var activityType string
			if err := rows.Scan(&it.Kind, &it.OccurredAt, &it.LeadID, &it.LeadName, &it.ActorUserID,
				&it.ActorName, &it.FromValue, &it.ToValue, &it.FromName, &it.ToName,
				&it.ActivityID, &activityType, &it.Subject); err != nil {
				rows.Close()
				return err
			}
			it.ActivityType = domain.ActivityType(activityType)
			d.RecentActivity = append(d.RecentActivity, it)
		}
		rows.Close()
		return rows.Err()
	})
	if err != nil {
		return domain.LeadDashboard{}, err
	}
	return d, nil
}
