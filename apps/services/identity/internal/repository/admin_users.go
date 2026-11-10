package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

var (
	// ErrCourierProfileRequired means a user would become a COURIER without a
	// courier profile.
	ErrCourierProfileRequired = errors.New("courier profile required")

	// ErrCourierProfileNotAllowed means courier profile fields were given for a
	// user who is not a COURIER.
	ErrCourierProfileNotAllowed = errors.New("courier profile not allowed for this role")
)

const (
	auditEntityUser = "user"

	auditUserCreated     = "USER_CREATED"
	auditUserUpdated     = "USER_UPDATED"
	auditUserActivated   = "USER_ACTIVATED"
	auditUserDeactivated = "USER_DEACTIVATED"
)

// CourierProfile is a courier's row in identity.courier_profiles.
type CourierProfile struct {
	VehicleType   *string
	VehicleNumber *string
	HomeZoneID    *string
	IsAvailable   bool
}

// AdminUser is the administrative view of a user (API v1.1 sections 2.10 to
// 2.13): the account, its role name and the courier profile if there is one.
type AdminUser struct {
	ID             string
	Email          string
	FullName       string
	Phone          *string
	Role           auth.Role
	IsActive       bool
	SessionVersion int
	CreatedAt      time.Time
	UpdatedAt      time.Time
	LastLoginAt    *time.Time
	Courier        *CourierProfile
}

// CourierProfileInput is the courier profile of a new staff account.
type CourierProfileInput struct {
	VehicleType   string
	VehicleNumber string
	HomeZoneID    string
}

// CreateStaffParams describes a staff account created by an administrator.
type CreateStaffParams struct {
	Email        string
	PasswordHash string
	FullName     string
	Phone        string
	Role         auth.Role
	Courier      *CourierProfileInput // set only when Role is COURIER
}

// CourierProfileChange is a partial courier profile update. A nil field is left
// as it is.
type CourierProfileChange struct {
	VehicleType   *string
	VehicleNumber *string
	HomeZoneID    *string
	IsAvailable   *bool
}

// UserChange is a partial update of a user. A nil field is left as it is.
type UserChange struct {
	FullName *string
	Phone    *string
	Role     *auth.Role
	Courier  *CourierProfileChange
}

// UserFilter selects and orders the admin user list.
type UserFilter struct {
	Role      auth.Role // empty for any role
	IsActive  *bool
	Search    string // matched against name and email, case-insensitively
	SortField string // createdAt, fullName or email
	SortDesc  bool
	Limit     int
	Offset    int
}

// CourierFilter selects the couriers offered for assignment.
type CourierFilter struct {
	IsAvailable *bool
	ZoneID      string // empty for any zone
	Limit       int
	Offset      int
}

// CourierListing is one row of the courier list (API v1.1 section 2.15).
type CourierListing struct {
	ID          string
	FullName    string
	Phone       *string
	VehicleType *string
	HomeZoneID  *string
	IsAvailable bool
}

// sortColumns maps the API sort fields to SQL. Only these values ever reach
// the query text, so a caller cannot inject SQL through sort.
var sortColumns = map[string]string{
	"createdAt": "u.created_at",
	"fullName":  "lower(u.full_name)",
	"email":     "lower(u.email)",
}

const adminUserSelect = `
	SELECT u.id, u.email, u.full_name, u.phone, r.name, u.is_active, u.session_version,
	       u.created_at, u.updated_at, u.last_login_at,
	       cp.user_id::text, cp.vehicle_type, cp.vehicle_number, cp.home_zone_id::text, cp.is_available
	FROM identity.users u
	JOIN identity.roles r ON r.id = u.role_id
	LEFT JOIN identity.courier_profiles cp ON cp.user_id = u.id`

// rowQuerier is satisfied by both the pool and a transaction.
type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func scanAdminUser(row pgx.Row) (AdminUser, error) {
	var (
		user      AdminUser
		role      string
		profileID *string
		profile   CourierProfile
		available *bool
	)
	if err := row.Scan(
		&user.ID, &user.Email, &user.FullName, &user.Phone, &role, &user.IsActive, &user.SessionVersion,
		&user.CreatedAt, &user.UpdatedAt, &user.LastLoginAt,
		&profileID, &profile.VehicleType, &profile.VehicleNumber, &profile.HomeZoneID, &available,
	); err != nil {
		return AdminUser{}, err
	}
	user.Role = auth.Role(role)
	if profileID != nil {
		profile.IsAvailable = available != nil && *available
		user.Courier = &profile
	}
	return user, nil
}

func loadAdminUser(ctx context.Context, q rowQuerier, userID string) (AdminUser, error) {
	user, err := scanAdminUser(q.QueryRow(ctx, adminUserSelect+` WHERE u.id = $1`, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return AdminUser{}, ErrNotFound
	}
	if err != nil {
		return AdminUser{}, fmt.Errorf("reading user: %w", err)
	}
	return user, nil
}

// AdminUserByID returns the administrative view of one user, or ErrNotFound.
func (r *Repository) AdminUserByID(ctx context.Context, userID string) (AdminUser, error) {
	return loadAdminUser(ctx, r.db, userID)
}

// ListAdminUsers returns one page of users matching filter and the total number
// of matches.
func (r *Repository) ListAdminUsers(ctx context.Context, filter UserFilter) ([]AdminUser, int, error) {
	where, args := userWhere(filter)

	var total int
	if err := r.db.QueryRow(ctx, `
		SELECT count(*)
		FROM identity.users u
		JOIN identity.roles r ON r.id = u.role_id`+where, args...,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting users: %w", err)
	}

	column, ok := sortColumns[filter.SortField]
	if !ok {
		column = sortColumns["createdAt"]
	}
	direction := "ASC"
	if filter.SortDesc {
		direction = "DESC"
	}
	args = append(args, filter.Limit, filter.Offset)
	query := adminUserSelect + where +
		" ORDER BY " + column + " " + direction + ", u.id" +
		" LIMIT $" + strconv.Itoa(len(args)-1) + " OFFSET $" + strconv.Itoa(len(args))

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing users: %w", err)
	}
	defer rows.Close()

	users := []AdminUser{}
	for rows.Next() {
		user, err := scanAdminUser(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("listing users: %w", err)
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("listing users: %w", err)
	}
	return users, total, nil
}

// userWhere builds the WHERE clause for filter. Values always travel as
// parameters.
func userWhere(filter UserFilter) (string, []any) {
	var (
		conditions []string
		args       []any
	)
	add := func(condition string, value any) {
		args = append(args, value)
		conditions = append(conditions, strings.ReplaceAll(condition, "?", "$"+strconv.Itoa(len(args))))
	}
	if filter.Role != "" {
		add("r.name = ?", string(filter.Role))
	}
	if filter.IsActive != nil {
		add("u.is_active = ?", *filter.IsActive)
	}
	if filter.Search != "" {
		add(`(u.full_name ILIKE ? ESCAPE '\' OR u.email ILIKE ? ESCAPE '\')`, "%"+escapeLike(filter.Search)+"%")
	}
	if len(conditions) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(conditions, " AND "), args
}

// escapeLike makes %, _ and \ match themselves in a LIKE pattern.
func escapeLike(value string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value)
}

// ListCouriers returns one page of active couriers and the total number of
// matches, ordered by name.
func (r *Repository) ListCouriers(ctx context.Context, filter CourierFilter) ([]CourierListing, int, error) {
	conditions := []string{"u.is_active", "r.name = 'COURIER'"}
	var args []any
	if filter.IsAvailable != nil {
		args = append(args, *filter.IsAvailable)
		conditions = append(conditions, "COALESCE(cp.is_available, false) = $"+strconv.Itoa(len(args)))
	}
	if filter.ZoneID != "" {
		args = append(args, filter.ZoneID)
		conditions = append(conditions, "cp.home_zone_id = $"+strconv.Itoa(len(args)))
	}
	from := `
		FROM identity.users u
		JOIN identity.roles r ON r.id = u.role_id
		LEFT JOIN identity.courier_profiles cp ON cp.user_id = u.id
		WHERE ` + strings.Join(conditions, " AND ")

	var total int
	if err := r.db.QueryRow(ctx, `SELECT count(*)`+from, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting couriers: %w", err)
	}

	args = append(args, filter.Limit, filter.Offset)
	rows, err := r.db.Query(ctx, `
		SELECT u.id, u.full_name, u.phone, cp.vehicle_type, cp.home_zone_id::text, COALESCE(cp.is_available, false)`+
		from+` ORDER BY lower(u.full_name), u.id LIMIT $`+strconv.Itoa(len(args)-1)+` OFFSET $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing couriers: %w", err)
	}
	defer rows.Close()

	couriers := []CourierListing{}
	for rows.Next() {
		var courier CourierListing
		if err := rows.Scan(&courier.ID, &courier.FullName, &courier.Phone, &courier.VehicleType, &courier.HomeZoneID, &courier.IsAvailable); err != nil {
			return nil, 0, fmt.Errorf("listing couriers: %w", err)
		}
		couriers = append(couriers, courier)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("listing couriers: %w", err)
	}
	return couriers, total, nil
}

// CreateStaffUser inserts a LOCAL staff account, its courier profile when the
// role is COURIER, and the audit row, in one transaction. It returns
// ErrEmailTaken when the email is already registered in any letter case.
func (r *Repository) CreateStaffUser(ctx context.Context, params CreateStaffParams, actorID string) (AdminUser, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return AdminUser{}, fmt.Errorf("creating staff user: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var userID string
	err = tx.QueryRow(ctx, `
		INSERT INTO identity.users (email, password_hash, full_name, phone, role_id, auth_provider)
		SELECT $1, $2, $3, $4, ro.id, 'LOCAL'
		FROM identity.roles ro
		WHERE ro.name = $5
		RETURNING id`,
		params.Email, params.PasswordHash, params.FullName, params.Phone, string(params.Role),
	).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return AdminUser{}, fmt.Errorf("creating staff user: unknown role %q", params.Role)
	}
	if isEmailTaken(err) {
		return AdminUser{}, ErrEmailTaken
	}
	if err != nil {
		return AdminUser{}, fmt.Errorf("creating staff user: %w", err)
	}

	if params.Courier != nil {
		if _, err := tx.Exec(ctx, `
			INSERT INTO identity.courier_profiles (user_id, vehicle_type, vehicle_number, home_zone_id)
			VALUES ($1, NULLIF($2, ''), NULLIF($3, ''), NULLIF($4, '')::uuid)`,
			userID, params.Courier.VehicleType, params.Courier.VehicleNumber, params.Courier.HomeZoneID,
		); err != nil {
			return AdminUser{}, fmt.Errorf("creating staff user: %w", err)
		}
	}

	if err := insertAudit(ctx, tx, actorID, auditUserCreated, userID, map[string]any{"role": string(params.Role)}); err != nil {
		return AdminUser{}, err
	}
	created, err := loadAdminUser(ctx, tx, userID)
	if err != nil {
		return AdminUser{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return AdminUser{}, fmt.Errorf("creating staff user: %w", err)
	}
	return created, nil
}

// UpdateAdminUser applies change to the user and writes the audit row, in one
// transaction. The user row is locked, so concurrent edits run one after the
// other. It returns ErrNotFound, ErrCourierProfileRequired when the user would
// be a COURIER without a profile, and ErrCourierProfileNotAllowed when profile
// fields are given for a user who is not a COURIER.
func (r *Repository) UpdateAdminUser(ctx context.Context, userID string, change UserChange, actorID string) (AdminUser, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return AdminUser{}, fmt.Errorf("updating user: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var locked string
	err = tx.QueryRow(ctx, `SELECT id FROM identity.users WHERE id = $1 FOR UPDATE`, userID).Scan(&locked)
	if errors.Is(err, pgx.ErrNoRows) {
		return AdminUser{}, ErrNotFound
	}
	if err != nil {
		return AdminUser{}, fmt.Errorf("updating user: %w", err)
	}
	before, err := loadAdminUser(ctx, tx, userID)
	if err != nil {
		return AdminUser{}, err
	}

	var role *string
	if change.Role != nil {
		value := string(*change.Role)
		role = &value
	}
	if _, err := tx.Exec(ctx, `
		UPDATE identity.users
		SET full_name = COALESCE($2, full_name),
		    phone     = COALESCE($3, phone),
		    role_id   = COALESCE((SELECT id FROM identity.roles WHERE name = $4), role_id)
		WHERE id = $1`,
		userID, change.FullName, change.Phone, role,
	); err != nil {
		return AdminUser{}, fmt.Errorf("updating user: %w", err)
	}

	if change.Courier != nil {
		c := change.Courier
		if _, err := tx.Exec(ctx, `
			INSERT INTO identity.courier_profiles (user_id, vehicle_type, vehicle_number, home_zone_id, is_available)
			VALUES ($1, $2, $3, $4::uuid, COALESCE($5, true))
			ON CONFLICT (user_id) DO UPDATE SET
			    vehicle_type   = COALESCE($2, identity.courier_profiles.vehicle_type),
			    vehicle_number = COALESCE($3, identity.courier_profiles.vehicle_number),
			    home_zone_id   = COALESCE($4::uuid, identity.courier_profiles.home_zone_id),
			    is_available   = COALESCE($5, identity.courier_profiles.is_available)`,
			userID, c.VehicleType, c.VehicleNumber, c.HomeZoneID, c.IsAvailable,
		); err != nil {
			return AdminUser{}, fmt.Errorf("updating user: %w", err)
		}
	}

	after, err := loadAdminUser(ctx, tx, userID)
	if err != nil {
		return AdminUser{}, err
	}
	if after.Role == auth.RoleCourier && after.Courier == nil {
		return AdminUser{}, ErrCourierProfileRequired
	}
	if change.Courier != nil && after.Role != auth.RoleCourier {
		return AdminUser{}, ErrCourierProfileNotAllowed
	}

	// The audit details name the fields that changed and the role before and
	// after. Phone numbers and other personal values are not copied into it.
	details := map[string]any{"changedFields": changedFields(change)}
	if before.Role != after.Role {
		details["before"] = map[string]any{"role": string(before.Role)}
		details["after"] = map[string]any{"role": string(after.Role)}
	}
	if err := insertAudit(ctx, tx, actorID, auditUserUpdated, userID, details); err != nil {
		return AdminUser{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return AdminUser{}, fmt.Errorf("updating user: %w", err)
	}
	return after, nil
}

func changedFields(change UserChange) []string {
	fields := []string{}
	if change.FullName != nil {
		fields = append(fields, "fullName")
	}
	if change.Phone != nil {
		fields = append(fields, "phone")
	}
	if change.Role != nil {
		fields = append(fields, "role")
	}
	if change.Courier != nil {
		fields = append(fields, "courierProfile")
	}
	return fields
}

// SetUserActive activates or deactivates the user and writes the audit row, in
// one transaction. Deactivation also revokes every active refresh token of the
// user, so existing sessions cannot be renewed. It returns ErrNotFound for an
// unknown user.
func (r *Repository) SetUserActive(ctx context.Context, userID string, active bool, actorID string, now time.Time) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("setting user status: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var wasActive bool
	err = tx.QueryRow(ctx, `SELECT is_active FROM identity.users WHERE id = $1 FOR UPDATE`, userID).Scan(&wasActive)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("setting user status: %w", err)
	}

	if _, err := tx.Exec(ctx, `UPDATE identity.users SET is_active = $2 WHERE id = $1`, userID, active); err != nil {
		return fmt.Errorf("setting user status: %w", err)
	}
	if !active {
		if _, err := tx.Exec(ctx, `
			UPDATE identity.refresh_tokens
			SET revoked_at = $2
			WHERE user_id = $1 AND revoked_at IS NULL`,
			userID, now.UTC(),
		); err != nil {
			return fmt.Errorf("setting user status: %w", err)
		}
	}

	action := auditUserDeactivated
	if active {
		action = auditUserActivated
	}
	details := map[string]any{
		"before": map[string]any{"isActive": wasActive},
		"after":  map[string]any{"isActive": active},
	}
	if err := insertAudit(ctx, tx, actorID, action, userID, details); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("setting user status: %w", err)
	}
	return nil
}

// insertAudit writes one identity.audit_logs row for an action on a user. The
// details must never hold secrets, tokens or passwords (DB v1.1 section 3.6).
func insertAudit(ctx context.Context, tx pgx.Tx, actorID, action, userID string, details map[string]any) error {
	payload, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("writing audit log: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO identity.audit_logs (actor_user_id, action, entity_type, entity_id, details)
		VALUES ($1, $2, $3, $4, $5::jsonb)`,
		actorID, action, auditEntityUser, userID, string(payload),
	); err != nil {
		return fmt.Errorf("writing audit log: %w", err)
	}
	return nil
}
