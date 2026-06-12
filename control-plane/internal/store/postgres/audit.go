package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/grid-computing/control-plane/internal/domain"
)

// WriteAuditEvent appends an immutable audit record.  This function only
// performs INSERT; no UPDATE or DELETE operations are permitted on the audit
// log.
func (s *Store) WriteAuditEvent(ctx context.Context, ev *domain.AuditEvent) error {
	detailsJSON, err := json.Marshal(ev.Details)
	if err != nil {
		return fmt.Errorf("audit: marshal details: %w", err)
	}

	const q = `
		INSERT INTO audit_log (
			id, org_unit_id, actor_type, actor_id, action,
			resource_type, resource_id, details, ip_address, occurred_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
	`
	_, err = s.pool.Exec(ctx, q,
		ev.ID, ev.OrgUnitID, ev.ActorType, ev.ActorID, ev.Action,
		ev.ResourceType, ev.ResourceID, detailsJSON, ev.IPAddress, ev.OccurredAt,
	)
	if err != nil {
		return fmt.Errorf("audit: write event: %w", err)
	}
	return nil
}

// QueryAuditLog returns paginated audit events for an org unit within the
// specified time range, ordered by occurred_at DESC.
func (s *Store) QueryAuditLog(
	ctx context.Context,
	orgUnitID uuid.UUID,
	from, to time.Time,
	limit, offset int,
) ([]*domain.AuditEvent, error) {
	const q = `
		SELECT id, org_unit_id, actor_type, actor_id, action,
		       resource_type, resource_id, details, ip_address, occurred_at
		FROM audit_log
		WHERE org_unit_id = $1
		  AND occurred_at >= $2
		  AND occurred_at <= $3
		ORDER BY occurred_at DESC
		LIMIT $4 OFFSET $5
	`
	rows, err := s.pool.Query(ctx, q, orgUnitID, from, to, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("audit: query: %w", err)
	}
	defer rows.Close()

	var events []*domain.AuditEvent
	for rows.Next() {
		ev, err := scanAuditEvent(rows)
		if err != nil {
			return nil, fmt.Errorf("audit: scan: %w", err)
		}
		events = append(events, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("audit: rows: %w", err)
	}
	return events, nil
}

func scanAuditEvent(row pgx.Row) (*domain.AuditEvent, error) {
	var ev domain.AuditEvent
	var detailsRaw []byte

	if err := row.Scan(
		&ev.ID, &ev.OrgUnitID, &ev.ActorType, &ev.ActorID, &ev.Action,
		&ev.ResourceType, &ev.ResourceID, &detailsRaw, &ev.IPAddress, &ev.OccurredAt,
	); err != nil {
		return nil, err
	}

	if len(detailsRaw) > 0 {
		if err := json.Unmarshal(detailsRaw, &ev.Details); err != nil {
			return nil, fmt.Errorf("unmarshal details: %w", err)
		}
	}
	return &ev, nil
}
