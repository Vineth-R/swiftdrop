package pricing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type DB interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
	Begin(context.Context) (pgx.Tx, error)
}
type Repository struct{ db DB }

func NewRepository(db DB) *Repository { return &Repository{db: db} }

func (r *Repository) Quote(ctx context.Context, input Input) (Quote, error) {
	if err := input.Validate(); err != nil {
		return Quote{}, err
	}
	if r.db == nil {
		return Quote{}, ErrUnavailable
	}
	var origin, destination *bool
	var base, maximum, weight, size, currency *string
	err := r.db.QueryRow(ctx, `
	 SELECT origin.is_active, destination.is_active, rule.base_fee::text,
	   (SELECT max(max_weight_kg)::text FROM pricing.weight_bands WHERE is_active),
	   band.charge::text, size.surcharge::text, rule.currency::text
	 FROM (SELECT 1) AS anchor
	 LEFT JOIN pricing.zones origin ON origin.id = $1
	 LEFT JOIN pricing.zones destination ON destination.id = $2
	 LEFT JOIN pricing.pricing_rules rule ON rule.origin_zone_id = origin.id
	   AND rule.destination_zone_id = destination.id AND rule.is_active
	 LEFT JOIN pricing.weight_bands band ON band.is_active
	   AND band.min_weight_kg < $3::numeric AND band.max_weight_kg >= $3::numeric
	 LEFT JOIN pricing.size_surcharges size ON size.size_category = $4`,
		input.OriginZoneID, input.DestinationZoneID, input.WeightKg.String(), input.SizeCategory,
	).Scan(&origin, &destination, &base, &maximum, &weight, &size, &currency)
	if err != nil {
		return Quote{}, fmt.Errorf("read pricing snapshot: %w", err)
	}
	if origin == nil || destination == nil {
		return Quote{}, ErrZoneNotFound
	}
	if !*origin || !*destination {
		return Quote{}, ErrZoneNotFound
	}
	if base == nil {
		return Quote{}, ErrRouteNotServiceable
	}
	if maximum != nil {
		maxWeight, err := ParseDecimal(*maximum)
		if err != nil {
			return Quote{}, fmt.Errorf("read maximum weight: %w", err)
		}
		if input.WeightKg > maxWeight {
			return Quote{}, ErrWeightNotPriced
		}
	}
	if currency == nil || strings.TrimSpace(*currency) != "LKR" {
		return Quote{}, errors.New("unsupported pricing currency configuration")
	}
	if weight == nil {
		return Quote{}, ErrWeightNotPriced
	}
	if size == nil {
		return Quote{}, errors.New("size surcharge configuration is missing")
	}
	amounts := make([]Decimal, 0, 3)
	for _, text := range []*string{base, weight, size} {
		amount, err := ParseDecimal(*text)
		if err != nil || amount < 0 {
			return Quote{}, errors.New("invalid pricing amount in database")
		}
		amounts = append(amounts, amount)
	}
	quote := Calculate(input, amounts[0], amounts[1], amounts[2])
	if quote.Total > 999999999999 {
		return Quote{}, errors.New("quote exceeds supported monetary precision")
	}
	return quote, nil
}

func (r *Repository) Zones(ctx context.Context) ([]Zone, error) {
	zones := []Zone{}
	if r.db == nil {
		return zones, ErrUnavailable
	}
	rows, err := r.db.Query(ctx, `SELECT id::text, code, name, coalesce(districts, '{}'), is_active
	 FROM pricing.zones WHERE is_active ORDER BY code, id`)
	if err != nil {
		return nil, fmt.Errorf("list zones: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var zone Zone
		if err := rows.Scan(&zone.ID, &zone.Code, &zone.Name, &zone.Districts, &zone.IsActive); err != nil {
			return nil, fmt.Errorf("scan zone: %w", err)
		}
		zones = append(zones, zone)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate zones: %w", err)
	}
	return zones, nil
}
func (r *Repository) Zone(ctx context.Context, id string) (Zone, error) {
	if !ValidID(id) {
		return Zone{}, &Invalid{Field: "zoneId", Issue: "must be a UUID"}
	}
	if r.db == nil {
		return Zone{}, ErrUnavailable
	}
	var zone Zone
	err := r.db.QueryRow(ctx, `SELECT id::text, code, name, coalesce(districts, '{}'), is_active
	 FROM pricing.zones WHERE id = $1`, id).Scan(&zone.ID, &zone.Code, &zone.Name, &zone.Districts, &zone.IsActive)
	if errors.Is(err, pgx.ErrNoRows) {
		return Zone{}, ErrZoneNotFound
	}
	if err != nil {
		return Zone{}, fmt.Errorf("read zone: %w", err)
	}
	return zone, nil
}

type Mutation struct {
	Code              *string   `json:"code,omitempty"`
	Name              *string   `json:"name,omitempty"`
	Districts         *[]string `json:"districts,omitempty"`
	OriginZoneID      *string   `json:"originZoneId,omitempty"`
	DestinationZoneID *string   `json:"destinationZoneId,omitempty"`
	BaseFee           *Decimal  `json:"baseFee,omitempty"`
	MinWeightKg       *Decimal  `json:"minWeightKg,omitempty"`
	MaxWeightKg       *Decimal  `json:"maxWeightKg,omitempty"`
	Charge            *Decimal  `json:"charge,omitempty"`
	Surcharge         *Decimal  `json:"surcharge,omitempty"`
	IsActive          *bool     `json:"isActive,omitempty"`
}
type resource struct{ table, key, projection string }

var resources = map[string]resource{
	"zones":      {table: "pricing.zones", key: "id", projection: `jsonb_build_object('id',id,'code',code,'name',name,'districts',coalesce(districts,'{}'),'isActive',is_active)`},
	"rules":      {table: "pricing.pricing_rules", key: "id", projection: `jsonb_build_object('id',id,'originZoneId',origin_zone_id,'destinationZoneId',destination_zone_id,'baseFee',base_fee,'currency',currency,'isActive',is_active,'updatedAt',updated_at)`},
	"bands":      {table: "pricing.weight_bands", key: "id", projection: `jsonb_build_object('id',id,'minWeightKg',min_weight_kg,'maxWeightKg',max_weight_kg,'charge',charge,'isActive',is_active)`},
	"surcharges": {table: "pricing.size_surcharges", key: "size_category", projection: `jsonb_build_object('sizeCategory',size_category,'surcharge',surcharge)`},
}

func (m Mutation) columns() ([]string, []any) {
	columns := []string{}
	values := []any{}
	add := func(column string, value any) { columns = append(columns, column); values = append(values, value) }
	if m.Code != nil {
		add("code", strings.TrimSpace(*m.Code))
	}
	if m.Name != nil {
		add("name", strings.TrimSpace(*m.Name))
	}
	if m.Districts != nil {
		add("districts", *m.Districts)
	}
	if m.OriginZoneID != nil {
		add("origin_zone_id", *m.OriginZoneID)
	}
	if m.DestinationZoneID != nil {
		add("destination_zone_id", *m.DestinationZoneID)
	}
	if m.BaseFee != nil {
		add("base_fee", m.BaseFee.String())
	}
	if m.MinWeightKg != nil {
		add("min_weight_kg", m.MinWeightKg.String())
	}
	if m.MaxWeightKg != nil {
		add("max_weight_kg", m.MaxWeightKg.String())
	}
	if m.Charge != nil {
		add("charge", m.Charge.String())
	}
	if m.Surcharge != nil {
		add("surcharge", m.Surcharge.String())
	}
	if m.IsActive != nil {
		add("is_active", *m.IsActive)
	}
	return columns, values
}

// Mutate keeps the configuration write and its audit entry in one transaction.
// Table/column SQL comes exclusively from the fixed resource and field allowlists.
func (r *Repository) Mutate(ctx context.Context, kind, id, actor string, m Mutation) (result json.RawMessage, err error) {
	if err := m.Validate(kind, id == ""); err != nil {
		return nil, err
	}
	if !ValidID(actor) {
		return nil, &Invalid{Field: "actor", Issue: "must be a UUID"}
	}
	spec, ok := resources[kind]
	if !ok {
		return nil, ErrNotFound
	}
	if r.db == nil {
		return nil, ErrUnavailable
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin pricing change: %w", err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if rollbackErr := tx.Rollback(cleanup); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			err = errors.Join(err, fmt.Errorf("rollback pricing change: %w", rollbackErr))
		}
	}()
	columns, values := m.columns()
	if kind == "rules" {
		columns = append(columns, "updated_by")
		values = append(values, actor)
	}
	creating := id == ""
	if kind == "rules" && creating {
		var active bool
		var count int
		if err := tx.QueryRow(ctx, `SELECT coalesce(bool_and(is_active),false), count(*) FROM pricing.zones WHERE id IN ($1,$2)`,
			*m.OriginZoneID, *m.DestinationZoneID).Scan(&active, &count); err != nil {
			return nil, fmt.Errorf("validate rule zones: %w", err)
		}
		wanted := 2
		if *m.OriginZoneID == *m.DestinationZoneID {
			wanted = 1
		}
		if !active || count != wanted {
			return nil, ErrZoneNotFound
		}
	}
	var raw string
	if creating {
		placeholders := make([]string, len(values))
		for index := range values {
			placeholders[index] = fmt.Sprintf("$%d", index+1)
		}
		query := "INSERT INTO " + spec.table + " (" + strings.Join(columns, ",") + ") VALUES (" +
			strings.Join(placeholders, ",") + ") RETURNING " + spec.projection + "::text"
		err = tx.QueryRow(ctx, query, values...).Scan(&raw)
	} else {
		var locked string
		err = tx.QueryRow(ctx, "SELECT "+spec.key+"::text FROM "+spec.table+" WHERE "+spec.key+"=$1 FOR UPDATE", id).Scan(&locked)
		if err == nil {
			assignments := make([]string, len(columns))
			for index, column := range columns {
				assignments[index] = fmt.Sprintf("%s=$%d", column, index+1)
			}
			values = append(values, id)
			query := "UPDATE " + spec.table + " SET " + strings.Join(assignments, ",") + " WHERE " + spec.key +
				fmt.Sprintf("=$%d RETURNING ", len(values)) + spec.projection + "::text"
			err = tx.QueryRow(ctx, query, values...).Scan(&raw)
		}
	}
	if err != nil {
		return nil, mapWriteError(err)
	}
	var entity struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(raw), &entity); err != nil {
		return nil, fmt.Errorf("decode changed pricing resource: %w", err)
	}
	var entityID any
	if entity.ID != "" {
		entityID = entity.ID
	}
	details, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("encode pricing audit: %w", err)
	}
	action := "updated"
	if creating {
		action = "created"
	}
	if _, err := tx.Exec(ctx, `INSERT INTO pricing.audit_logs(actor_user_id,action,entity_type,entity_id,details)
	 VALUES ($1,$2,$3,$4,$5::jsonb)`, actor, "pricing."+kind+"."+action, kind, entityID, string(details)); err != nil {
		return nil, fmt.Errorf("write pricing audit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit pricing change: %w", err)
	}
	return json.RawMessage(raw), nil
}

func mapWriteError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgError *pgconn.PgError
	if errors.As(err, &pgError) {
		switch pgError.ConstraintName {
		case "zones_code_key":
			return &Conflict{Code: "ZONE_CODE_EXISTS"}
		case "pricing_rules_active_pair_uq":
			return &Conflict{Code: "RULE_EXISTS"}
		case "weight_bands_no_overlap":
			return &Conflict{Code: "BAND_OVERLAP"}
		}
		if pgError.Code == "23514" || pgError.Code == "22003" {
			return &Invalid{Field: "request", Issue: "invalid pricing range or amount"}
		}
		if pgError.Code == "23503" {
			return ErrZoneNotFound
		}
	}
	return fmt.Errorf("change pricing configuration: %w", err)
}

type Conflict struct{ Code string }

func (e *Conflict) Error() string { return e.Code }

type Filter struct {
	Page, PageSize      int
	Origin, Destination string
	Active              *bool
	Sort                string
}

func (r *Repository) List(ctx context.Context, kind string, filter Filter) ([]json.RawMessage, int, error) {
	result := []json.RawMessage{}
	spec, ok := resources[kind]
	if !ok {
		return nil, 0, ErrNotFound
	}
	if r.db == nil {
		return nil, 0, ErrUnavailable
	}
	if filter.Page < 1 || filter.Page > 10000 || filter.PageSize < 1 || filter.PageSize > 100 {
		return nil, 0, &Invalid{Field: "pagination", Issue: "invalid pagination bounds"}
	}
	order := spec.key + " ASC"
	switch filter.Sort {
	case "", "id:asc":
	case "id:desc":
		order = spec.key + " DESC"
	case "createdAt:asc", "createdAt:desc":
		if kind == "surcharges" {
			return nil, 0, &Invalid{Field: "sort", Issue: "unsupported sort"}
		}
		order = "created_at " + strings.ToUpper(strings.Split(filter.Sort, ":")[1]) + ", " + spec.key + " ASC"
	case "updatedAt:asc", "updatedAt:desc":
		order = "updated_at " + strings.ToUpper(strings.Split(filter.Sort, ":")[1]) + ", " + spec.key + " ASC"
	default:
		return nil, 0, &Invalid{Field: "sort", Issue: "unsupported sort"}
	}
	where := []string{"true"}
	args := []any{}
	if kind == "rules" {
		for _, field := range []struct{ column, value string }{{column: "origin_zone_id", value: filter.Origin}, {column: "destination_zone_id", value: filter.Destination}} {
			if field.value != "" {
				args = append(args, field.value)
				where = append(where, fmt.Sprintf("%s=$%d", field.column, len(args)))
			}
		}
		if filter.Active != nil {
			args = append(args, *filter.Active)
			where = append(where, fmt.Sprintf("is_active=$%d", len(args)))
		}
	}
	clause := " FROM " + spec.table + " WHERE " + strings.Join(where, " AND ")
	var total int
	if err := r.db.QueryRow(ctx, "SELECT count(*)"+clause, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count pricing configuration: %w", err)
	}
	args = append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)
	query := "SELECT " + spec.projection + "::text" + clause + " ORDER BY " + order +
		fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)-1, len(args))
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list pricing configuration: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, 0, fmt.Errorf("scan pricing configuration: %w", err)
		}
		result = append(result, json.RawMessage(raw))
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate pricing configuration: %w", err)
	}
	return result, total, nil
}
