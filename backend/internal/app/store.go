package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

func (s *Store) EnsureSchema(ctx context.Context) error {
	stmts := []string{
		`CREATE EXTENSION IF NOT EXISTS pgcrypto`,
		`CREATE TABLE IF NOT EXISTS users (
			id uuid PRIMARY KEY,
			name text NOT NULL,
			role text NOT NULL CHECK (role IN ('officer', 'member')),
			plate text,
			balance_cents bigint NOT NULL DEFAULT 0,
			created_at timestamptz NOT NULL DEFAULT now()
		)`,
		`CREATE TABLE IF NOT EXISTS rule_versions (
			id uuid PRIMARY KEY,
			version integer NOT NULL UNIQUE,
			active boolean NOT NULL DEFAULT false,
			rules jsonb NOT NULL,
			created_at timestamptz NOT NULL DEFAULT now()
		)`,
		`CREATE TABLE IF NOT EXISTS violations (
			id uuid PRIMARY KEY,
			plate text NOT NULL,
			violation_type text NOT NULL,
			location text NOT NULL,
			occurred_at timestamptz NOT NULL,
			photo_data text NOT NULL,
			fine_cents bigint NOT NULL,
			rule_version_id uuid NOT NULL REFERENCES rule_versions(id),
			rule_version_number integer NOT NULL,
			rule_snapshot jsonb NOT NULL,
			prior_unpaid_count integer NOT NULL,
			time_multiplier numeric(5,2) NOT NULL,
			repeat_multiplier numeric(5,2) NOT NULL,
			status text NOT NULL DEFAULT 'unpaid',
			owner_user_id uuid REFERENCES users(id),
			created_at timestamptz NOT NULL DEFAULT now()
		)`,
		`CREATE TABLE IF NOT EXISTS payments (
			id uuid PRIMARY KEY,
			violation_id uuid NOT NULL UNIQUE REFERENCES violations(id),
			member_user_id uuid NOT NULL REFERENCES users(id),
			amount_cents bigint NOT NULL,
			scenario text NOT NULL,
			status text NOT NULL,
			transaction_id text NOT NULL,
			created_at timestamptz NOT NULL DEFAULT now()
		)`,
		`CREATE TABLE IF NOT EXISTS notifications (
			id uuid PRIMARY KEY,
			violation_id uuid NOT NULL REFERENCES violations(id),
			user_id uuid NOT NULL REFERENCES users(id),
			kind text NOT NULL,
			message text NOT NULL,
			status text NOT NULL,
			created_at timestamptz NOT NULL DEFAULT now()
		)`,
	}
	for _, stmt := range stmts {
		if _, err := s.pool.Exec(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Seed(ctx context.Context, officerID, memberID string, memberPlate string, memberBalance int64) error {
	var userCount int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&userCount); err != nil {
		return err
	}
	if userCount == 0 {
		_, err := s.pool.Exec(ctx, `INSERT INTO users (id, name, role, plate, balance_cents)
			VALUES ($1, $2, $3, NULL, 0), ($4, $5, $6, $7, $8)`,
			officerID, "Officer One", RoleOfficer, memberID, "Member One", RoleMember, strings.ToUpper(strings.ReplaceAll(memberPlate, " ", "")), memberBalance,
		)
		if err != nil {
			return err
		}
	}
	var ruleCount int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM rule_versions`).Scan(&ruleCount); err != nil {
		return err
	}
	if ruleCount == 0 {
		rules := DefaultFineRules()
		rulesJSON, err := json.Marshal(rules)
		if err != nil {
			return err
		}
		_, err = s.pool.Exec(ctx, `INSERT INTO rule_versions (id, version, active, rules) VALUES ($1, 1, true, $2)`, uuid.NewString(), rulesJSON)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, name, role, COALESCE(plate, ''), balance_cents, created_at FROM users ORDER BY role, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := make([]User, 0)
	for rows.Next() {
		var user User
		var createdAt time.Time
		if err := rows.Scan(&user.ID, &user.Name, &user.Role, &user.Plate, &user.BalanceCents, &createdAt); err != nil {
			return nil, err
		}
		user.CreatedAt = createdAt.Format(time.RFC3339)
		users = append(users, user)
	}
	return users, rows.Err()
}

func (s *Store) GetUser(ctx context.Context, userID string) (*User, error) {
	var user User
	var plate *string
	var createdAt time.Time
	err := s.pool.QueryRow(ctx, `SELECT id, name, role, plate, balance_cents, created_at FROM users WHERE id = $1`, userID).
		Scan(&user.ID, &user.Name, &user.Role, &plate, &user.BalanceCents, &createdAt)
	if err != nil {
		return nil, err
	}
	if plate != nil {
		user.Plate = *plate
	}
	user.CreatedAt = createdAt.Format(time.RFC3339)
	return &user, nil
}

func (s *Store) GetMemberByPlate(ctx context.Context, plate string) (*User, error) {
	var user User
	var createdAt time.Time
	err := s.pool.QueryRow(ctx, `SELECT id, name, role, plate, balance_cents, created_at FROM users WHERE role = $1 AND upper(regexp_replace(coalesce(plate, ''), '\\s+', '', 'g')) = upper(regexp_replace($2, '\\s+', '', 'g')) LIMIT 1`, RoleMember, plate).
		Scan(&user.ID, &user.Name, &user.Role, &user.Plate, &user.BalanceCents, &createdAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	user.CreatedAt = createdAt.Format(time.RFC3339)
	return &user, nil
}

func (s *Store) ActiveRuleVersion(ctx context.Context) (*RuleVersion, error) {
	var rv RuleVersion
	var rulesJSON []byte
	var createdAt time.Time
	err := s.pool.QueryRow(ctx, `SELECT id, version, active, rules, created_at FROM rule_versions WHERE active = true ORDER BY version DESC LIMIT 1`).Scan(&rv.ID, &rv.Version, &rv.Active, &rulesJSON, &createdAt)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(rulesJSON, &rv.Rules); err != nil {
		return nil, err
	}
	rv.CreatedAt = createdAt.Format(time.RFC3339)
	return &rv, nil
}

func (s *Store) ListRuleVersions(ctx context.Context) ([]RuleVersion, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, version, active, rules, created_at FROM rule_versions ORDER BY version DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	versions := make([]RuleVersion, 0)
	for rows.Next() {
		var rv RuleVersion
		var rulesJSON []byte
		var createdAt time.Time
		if err := rows.Scan(&rv.ID, &rv.Version, &rv.Active, &rulesJSON, &createdAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(rulesJSON, &rv.Rules); err != nil {
			return nil, err
		}
		rv.CreatedAt = createdAt.Format(time.RFC3339)
		versions = append(versions, rv)
	}
	return versions, rows.Err()
}

func (s *Store) PublishRuleVersion(ctx context.Context, rules FineRules) (*RuleVersion, error) {
	if err := rules.Validate(); err != nil {
		return nil, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	var nextVersion int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(version), 0) + 1 FROM rule_versions`).Scan(&nextVersion); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE rule_versions SET active = false WHERE active = true`); err != nil {
		return nil, err
	}
	rulesJSON, err := json.Marshal(rules)
	if err != nil {
		return nil, err
	}
	id := uuid.NewString()
	var createdAt time.Time
	if err := tx.QueryRow(ctx, `INSERT INTO rule_versions (id, version, active, rules) VALUES ($1, $2, true, $3) RETURNING created_at`, id, nextVersion, rulesJSON).Scan(&createdAt); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &RuleVersion{ID: id, Version: nextVersion, Active: true, Rules: rules, CreatedAt: createdAt.Format(time.RFC3339)}, nil
}

func (s *Store) CountPriorUnpaidViolations(ctx context.Context, plate string, occurredAt time.Time) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM violations
		WHERE upper(regexp_replace(plate, '\\s+', '', 'g')) = upper(regexp_replace($1, '\\s+', '', 'g'))
		AND status = 'unpaid'
		AND occurred_at >= $2 - INTERVAL '90 days'
		AND occurred_at < $2`, plate, occurredAt).Scan(&count)
	return count, err
}

func (s *Store) CreateViolation(ctx context.Context, input ViolationInput) (*Violation, *Notification, error) {
	occurredAt, err := time.Parse(time.RFC3339, input.OccurredAt)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid occurred_at: %w", err)
	}
	activeRule, err := s.ActiveRuleVersion(ctx)
	if err != nil {
		return nil, nil, err
	}
	priorCount, err := s.CountPriorUnpaidViolations(ctx, input.Plate, occurredAt)
	if err != nil {
		return nil, nil, err
	}
	fineCents, timeMultiplier, repeatMultiplier, err := CalculateFine(activeRule.Rules, input.ViolationType, occurredAt, priorCount)
	if err != nil {
		return nil, nil, err
	}
	owner, err := s.GetMemberByPlate(ctx, input.Plate)
	if err != nil {
		return nil, nil, err
	}
	rulesJSON, err := json.Marshal(activeRule.Rules)
	if err != nil {
		return nil, nil, err
	}
	violationID := uuid.NewString()
	var ownerID any
	if owner != nil {
		ownerID = owner.ID
	}
	var createdAt time.Time
	if err := s.pool.QueryRow(ctx, `INSERT INTO violations (
		id, plate, violation_type, location, occurred_at, photo_data, fine_cents,
		rule_version_id, rule_version_number, rule_snapshot, prior_unpaid_count,
		time_multiplier, repeat_multiplier, status, owner_user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		RETURNING created_at`,
		violationID, strings.ToUpper(strings.ReplaceAll(input.Plate, " ", "")), input.ViolationType, input.Location, occurredAt, input.PhotoData, fineCents,
		activeRule.ID, activeRule.Version, rulesJSON, priorCount, timeMultiplier, repeatMultiplier, ViolationUnpaid, ownerID,
	).Scan(&createdAt); err != nil {
		return nil, nil, err
	}
	message := fmt.Sprintf("Violation %s issued for %s with fine IDR %s.", violationID, strings.ToUpper(strings.ReplaceAll(input.Plate, " ", "")), formatIDR(fineCents))
	notification, err := s.createNotification(ctx, violationID, owner, "invoice_created", message)
	if err != nil {
		return nil, nil, err
	}
	return &Violation{
		ID:               violationID,
		Plate:            strings.ToUpper(strings.ReplaceAll(input.Plate, " ", "")),
		ViolationType:    input.ViolationType,
		Location:         input.Location,
		OccurredAt:       occurredAt.Format(time.RFC3339),
		PhotoData:        input.PhotoData,
		FineCents:        fineCents,
		RuleVersionID:    activeRule.ID,
		RuleVersionNumber: activeRule.Version,
		RuleSnapshot:     activeRule.Rules,
		PriorUnpaidCount: priorCount,
		TimeMultiplier:   timeMultiplier,
		RepeatMultiplier: repeatMultiplier,
		Status:           ViolationUnpaid,
		OwnerUserID:      ownerIDToString(ownerID),
		CreatedAt:        createdAt.Format(time.RFC3339),
	}, notification, nil
}

func (s *Store) ListViolations(ctx context.Context, user *User) ([]Violation, error) {
	query := `SELECT v.id, v.plate, v.violation_type, v.location, v.occurred_at, v.photo_data, v.fine_cents, v.rule_version_id, v.rule_version_number, v.rule_snapshot, v.prior_unpaid_count, v.time_multiplier, v.repeat_multiplier, v.status, v.owner_user_id, v.created_at, p.id, p.violation_id, p.member_user_id, p.amount_cents, p.scenario, p.status, p.transaction_id, p.created_at
	FROM violations v
	LEFT JOIN payments p ON p.violation_id = v.id`
	args := []any{}
	if user != nil && user.Role == RoleMember {
		query += ` WHERE v.owner_user_id = $1`
		args = append(args, user.ID)
	}
	query += ` ORDER BY v.created_at DESC`
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	violations := make([]Violation, 0)
	for rows.Next() {
		var violation Violation
		var occurredAt, createdAt time.Time
		var ruleSnapshotJSON []byte
		var ownerID *string
		var paymentID, paymentViolationID, paymentMemberID, paymentScenario, paymentStatus, paymentTransactionID *string
		var paymentAmount *int64
		var paymentCreatedAt *time.Time
		if err := rows.Scan(
			&violation.ID, &violation.Plate, &violation.ViolationType, &violation.Location, &occurredAt, &violation.PhotoData, &violation.FineCents,
			&violation.RuleVersionID, &violation.RuleVersionNumber, &ruleSnapshotJSON, &violation.PriorUnpaidCount, &violation.TimeMultiplier,
			&violation.RepeatMultiplier, &violation.Status, &ownerID, &createdAt,
			&paymentID, &paymentViolationID, &paymentMemberID, &paymentAmount, &paymentScenario, &paymentStatus, &paymentTransactionID, &paymentCreatedAt,
		); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(ruleSnapshotJSON, &violation.RuleSnapshot); err != nil {
			return nil, err
		}
		violation.OccurredAt = occurredAt.Format(time.RFC3339)
		violation.CreatedAt = createdAt.Format(time.RFC3339)
		violation.OwnerUserID = ownerID
		if paymentID != nil && paymentViolationID != nil && paymentMemberID != nil && paymentAmount != nil && paymentScenario != nil && paymentStatus != nil && paymentTransactionID != nil && paymentCreatedAt != nil {
			payment := &PaymentRecord{
				ID:            *paymentID,
				ViolationID:   *paymentViolationID,
				MemberUserID:  *paymentMemberID,
				AmountCents:   *paymentAmount,
				Scenario:      PaymentScenario(*paymentScenario),
				Status:        PaymentStatus(*paymentStatus),
				TransactionID: *paymentTransactionID,
				CreatedAt:     paymentCreatedAt.Format(time.RFC3339),
			}
			violation.PaymentTransaction = payment
		}
		violations = append(violations, violation)
	}
	return violations, rows.Err()
}

func (s *Store) GetViolation(ctx context.Context, violationID string) (*Violation, error) {
	violations, err := s.ListViolationsByIDs(ctx, []string{violationID})
	if err != nil {
		return nil, err
	}
	if len(violations) == 0 {
		return nil, pgx.ErrNoRows
	}
	return &violations[0], nil
}

func (s *Store) ListViolationsByIDs(ctx context.Context, ids []string) ([]Violation, error) {
	if len(ids) == 0 {
		return []Violation{}, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}
	query := fmt.Sprintf(`SELECT v.id, v.plate, v.violation_type, v.location, v.occurred_at, v.photo_data, v.fine_cents, v.rule_version_id, v.rule_version_number, v.rule_snapshot, v.prior_unpaid_count, v.time_multiplier, v.repeat_multiplier, v.status, v.owner_user_id, v.created_at, p.id, p.violation_id, p.member_user_id, p.amount_cents, p.scenario, p.status, p.transaction_id, p.created_at
	FROM violations v
	LEFT JOIN payments p ON p.violation_id = v.id
	WHERE v.id IN (%s)
	ORDER BY v.created_at DESC`, strings.Join(placeholders, ","))
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanViolations(rows)
}

func (s *Store) GetPaymentByViolationID(ctx context.Context, violationID string) (*PaymentRecord, error) {
	var payment PaymentRecord
	var createdAt time.Time
	err := s.pool.QueryRow(ctx, `SELECT id, violation_id, member_user_id, amount_cents, scenario, status, transaction_id, created_at FROM payments WHERE violation_id = $1`, violationID).
		Scan(&payment.ID, &payment.ViolationID, &payment.MemberUserID, &payment.AmountCents, &payment.Scenario, &payment.Status, &payment.TransactionID, &createdAt)
	if err != nil {
		return nil, err
	}
	payment.CreatedAt = createdAt.Format(time.RFC3339)
	return &payment, nil
}

func (s *Store) ListNotifications(ctx context.Context) ([]Notification, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, violation_id, user_id, kind, message, status, created_at FROM notifications ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	notifications := make([]Notification, 0)
	for rows.Next() {
		var notification Notification
		var createdAt time.Time
		if err := rows.Scan(&notification.ID, &notification.ViolationID, &notification.UserID, &notification.Kind, &notification.Message, &notification.Status, &createdAt); err != nil {
			return nil, err
		}
		notification.CreatedAt = createdAt.Format(time.RFC3339)
		notifications = append(notifications, notification)
	}
	return notifications, rows.Err()
}

func (s *Store) RecordPayment(ctx context.Context, violationID string, memberUser *User, amountCents int64, scenario PaymentScenario, status PaymentStatus, transactionID string) (*PaymentRecord, error) {
	if memberUser == nil {
		return nil, fmt.Errorf("member user is required")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	var violationOwnerID *string
	var violationStatus string
	var fineCents int64
	if err := tx.QueryRow(ctx, `SELECT owner_user_id, status, fine_cents FROM violations WHERE id = $1 FOR UPDATE`, violationID).Scan(&violationOwnerID, &violationStatus, &fineCents); err != nil {
		return nil, err
	}
	if violationOwnerID == nil || *violationOwnerID != memberUser.ID {
		return nil, fmt.Errorf("violation does not belong to member")
	}
	if violationStatus == string(ViolationPaid) {
		return nil, fmt.Errorf("violation already paid")
	}
	if amountCents != fineCents {
		return nil, fmt.Errorf("amount mismatch")
	}
	if status == PaymentPaid {
		if memberUser.BalanceCents < amountCents {
			return nil, fmt.Errorf("insufficient account balance")
		}
		if _, err := tx.Exec(ctx, `UPDATE users SET balance_cents = balance_cents - $1 WHERE id = $2`, amountCents, memberUser.ID); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `UPDATE violations SET status = $1 WHERE id = $2`, ViolationPaid, violationID); err != nil {
			return nil, err
		}
	}
	paymentID := uuid.NewString()
	var createdAt time.Time
	if err := tx.QueryRow(ctx, `INSERT INTO payments (id, violation_id, member_user_id, amount_cents, scenario, status, transaction_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING created_at`,
		paymentID, violationID, memberUser.ID, amountCents, scenario, status, transactionID).Scan(&createdAt); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &PaymentRecord{
		ID:            paymentID,
		ViolationID:   violationID,
		MemberUserID:  memberUser.ID,
		AmountCents:   amountCents,
		Scenario:      scenario,
		Status:        status,
		TransactionID: transactionID,
		CreatedAt:     createdAt.Format(time.RFC3339),
	}, nil
}

func (s *Store) UpdateBalance(ctx context.Context, userID string, balanceCents int64) error {
	_, err := s.pool.Exec(ctx, `UPDATE users SET balance_cents = $1 WHERE id = $2`, balanceCents, userID)
	return err
}

func (s *Store) createNotification(ctx context.Context, violationID string, owner *User, kind, message string) (*Notification, error) {
	if owner == nil {
		return nil, nil
	}
	notificationID := uuid.NewString()
	var createdAt time.Time
	if err := s.pool.QueryRow(ctx, `INSERT INTO notifications (id, violation_id, user_id, kind, message, status)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING created_at`, notificationID, violationID, owner.ID, kind, message, "sent").Scan(&createdAt); err != nil {
		return nil, err
	}
	return &Notification{ID: notificationID, ViolationID: violationID, UserID: owner.ID, Kind: kind, Message: message, Status: "sent", CreatedAt: createdAt.Format(time.RFC3339)}, nil
}

func ownerIDToString(owner any) *string {
	switch value := owner.(type) {
	case nil:
		return nil
	case string:
		return &value
	default:
		return nil
	}
}

func scanViolations(rows pgx.Rows) ([]Violation, error) {
	violations := make([]Violation, 0)
	for rows.Next() {
		var violation Violation
		var occurredAt, createdAt time.Time
		var ruleSnapshotJSON []byte
		var ownerID *string
		var paymentID, paymentViolationID, paymentMemberID, paymentScenario, paymentStatus, paymentTransactionID *string
		var paymentAmount *int64
		var paymentCreatedAt *time.Time
		if err := rows.Scan(
			&violation.ID, &violation.Plate, &violation.ViolationType, &violation.Location, &occurredAt, &violation.PhotoData, &violation.FineCents,
			&violation.RuleVersionID, &violation.RuleVersionNumber, &ruleSnapshotJSON, &violation.PriorUnpaidCount, &violation.TimeMultiplier,
			&violation.RepeatMultiplier, &violation.Status, &ownerID, &createdAt,
			&paymentID, &paymentViolationID, &paymentMemberID, &paymentAmount, &paymentScenario, &paymentStatus, &paymentTransactionID, &paymentCreatedAt,
		); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(ruleSnapshotJSON, &violation.RuleSnapshot); err != nil {
			return nil, err
		}
		violation.OccurredAt = occurredAt.Format(time.RFC3339)
		violation.CreatedAt = createdAt.Format(time.RFC3339)
		violation.OwnerUserID = ownerID
		if paymentID != nil && paymentViolationID != nil && paymentMemberID != nil && paymentAmount != nil && paymentScenario != nil && paymentStatus != nil && paymentTransactionID != nil && paymentCreatedAt != nil {
			violation.PaymentTransaction = &PaymentRecord{
				ID:            *paymentID,
				ViolationID:   *paymentViolationID,
				MemberUserID:  *paymentMemberID,
				AmountCents:   *paymentAmount,
				Scenario:      PaymentScenario(*paymentScenario),
				Status:        PaymentStatus(*paymentStatus),
				TransactionID: *paymentTransactionID,
				CreatedAt:     paymentCreatedAt.Format(time.RFC3339),
			}
		}
		violations = append(violations, violation)
	}
	return violations, rows.Err()
}

func formatIDR(value int64) string {
	negative := value < 0
	if negative {
		value = -value
	}
	parts := make([]string, 0)
	for value >= 1000 {
		parts = append([]string{fmt.Sprintf("%03d", value%1000)}, parts...)
		value /= 1000
	}
	parts = append([]string{fmt.Sprintf("%d", value)}, parts...)
	joined := strings.Join(parts, ".")
	if negative {
		return "-" + joined
	}
	return joined
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}
