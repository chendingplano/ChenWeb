package priceshandler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// decimalPattern accepts plain non-negative decimals such as 4, 0.04 or 2.00.
var decimalPattern = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`)

var (
	ErrInvalid  = errors.New("invalid price definition")
	ErrNotFound = errors.New("price definition not found")
)

type Item struct {
	ID       int64  `json:"id"`
	ItemName string `json:"item_name"`
	ItemType string `json:"item_type"`
	Cache    string `json:"cache"`
	TimeSpan string `json:"time_span"`
	Unit     string `json:"unit"`
	Currency string `json:"currency"`
	// Value is a decimal string so prices keep their exact precision end to end.
	Value string `json:"value"`
}

type PriceDef struct {
	ID           int64  `json:"id"`
	PriceDefName string `json:"price_def_name"`
	PriceType    string `json:"price_type"`
	Description  string `json:"description"`
	Items        []Item `json:"items"`
}

func validate(p *PriceDef) error {
	p.PriceDefName = strings.TrimSpace(p.PriceDefName)
	if p.PriceDefName == "" {
		return fmt.Errorf("%w: price definition name is required", ErrInvalid)
	}
	if p.PriceType != "service" && p.PriceType != "llm" {
		return fmt.Errorf("%w: price type must be service or llm", ErrInvalid)
	}
	if len(p.Items) == 0 {
		return fmt.Errorf("%w: at least one price item is required", ErrInvalid)
	}
	names := make(map[string]bool, len(p.Items))
	for i := range p.Items {
		item := &p.Items[i]
		n := i + 1
		item.ItemName = strings.TrimSpace(item.ItemName)
		item.Unit = strings.TrimSpace(item.Unit)
		item.Currency = strings.ToUpper(strings.TrimSpace(item.Currency))
		item.Value = strings.TrimSpace(item.Value)
		if item.ItemName == "" {
			return fmt.Errorf("%w: item %d needs a name", ErrInvalid, n)
		}
		if names[item.ItemName] {
			return fmt.Errorf("%w: item name %q is used more than once", ErrInvalid, item.ItemName)
		}
		names[item.ItemName] = true
		if item.ItemType != "input" && item.ItemType != "output" {
			return fmt.Errorf("%w: item %d has an invalid type", ErrInvalid, n)
		}
		if item.Cache != "" && item.Cache != "hit" && item.Cache != "miss" {
			return fmt.Errorf("%w: item %d has an invalid cache value", ErrInvalid, n)
		}
		if item.TimeSpan != "" && item.TimeSpan != "peak" && item.TimeSpan != "off-peak" {
			return fmt.Errorf("%w: item %d has an invalid time span", ErrInvalid, n)
		}
		if item.Unit == "" || item.Currency == "" {
			return fmt.Errorf("%w: item %d needs a unit and a currency", ErrInvalid, n)
		}
		if !decimalPattern.MatchString(item.Value) {
			return fmt.Errorf("%w: item %d needs a non-negative decimal value", ErrInvalid, n)
		}
	}
	return nil
}

func List(ctx context.Context, db *sql.DB) ([]PriceDef, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT d.id, d.price_def_name, d.price_type, d.description,
		       i.id, i.item_name, i.item_type, i.cache, i.time_span, i.unit, i.currency, i.value::text
		FROM public.price_defs d LEFT JOIN public.price_items i ON i.price_def_id = d.id
		ORDER BY d.price_type, d.price_def_name, d.id, i.sort_order, i.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]PriceDef, 0)
	for rows.Next() {
		var p PriceDef
		var id sql.NullInt64
		var name, typ, cache, span, unit, currency, value sql.NullString
		if err := rows.Scan(&p.ID, &p.PriceDefName, &p.PriceType, &p.Description,
			&id, &name, &typ, &cache, &span, &unit, &currency, &value); err != nil {
			return nil, err
		}
		if len(result) == 0 || result[len(result)-1].ID != p.ID {
			p.Items = []Item{}
			result = append(result, p)
		}
		if id.Valid {
			last := &result[len(result)-1]
			last.Items = append(last.Items, Item{ID: id.Int64, ItemName: name.String, ItemType: typ.String,
				Cache: cache.String, TimeSpan: span.String, Unit: unit.String, Currency: currency.String,
				Value: value.String})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func Save(ctx context.Context, db *sql.DB, id int64, p PriceDef) (int64, error) {
	if err := validate(&p); err != nil {
		return 0, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if id == 0 {
		err = tx.QueryRowContext(ctx, `INSERT INTO public.price_defs
			(price_def_name, price_type, description) VALUES ($1, $2, $3) RETURNING id`,
			p.PriceDefName, p.PriceType, p.Description).Scan(&id)
	} else {
		var updatedID int64
		err = tx.QueryRowContext(ctx, `UPDATE public.price_defs SET price_def_name=$2, price_type=$3,
			description=$4, updated_at=NOW() WHERE id=$1 RETURNING id`,
			id, p.PriceDefName, p.PriceType, p.Description).Scan(&updatedID)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrNotFound
		}
		if err == nil {
			_, err = tx.ExecContext(ctx, `DELETE FROM public.price_items WHERE price_def_id=$1`, id)
		}
	}
	if err != nil {
		return 0, err
	}
	for i, item := range p.Items {
		_, err = tx.ExecContext(ctx, `INSERT INTO public.price_items
			(price_def_id, item_name, item_type, cache, time_span, unit, currency, value, sort_order)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8::numeric,$9)`, id, item.ItemName, item.ItemType, item.Cache,
			item.TimeSpan, item.Unit, item.Currency, item.Value, i)
		if err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

func Delete(ctx context.Context, db *sql.DB, id int64) error {
	res, err := db.ExecContext(ctx, `DELETE FROM public.price_defs WHERE id=$1`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
