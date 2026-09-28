package releaseshandler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

var (
	ErrInvalid  = errors.New("invalid release")
	ErrNotFound = errors.New("release not found")
)

type Item struct {
	ID          int64  `json:"id"`
	ItemType    string `json:"item_type"`
	Description string `json:"description"`
	Notes       string `json:"notes"`
	PullRequest string `json:"pull_request"`
	TicketNum   string `json:"ticket_num"`
}

type Release struct {
	ID           int64  `json:"id"`
	MajorVersion string `json:"major_version"`
	MinorVersion string `json:"minor_version"`
	ReleaseNotes string `json:"release_notes"`
	ReleaseDate  string `json:"release_date"`
	Items        []Item `json:"items"`
}

func validate(r *Release) error {
	r.MajorVersion = strings.TrimSpace(r.MajorVersion)
	r.MinorVersion = strings.TrimSpace(r.MinorVersion)
	r.ReleaseDate = strings.TrimSpace(r.ReleaseDate)
	if r.MajorVersion == "" || r.MinorVersion == "" {
		return fmt.Errorf("%w: major and minor versions are required", ErrInvalid)
	}
	date, err := time.Parse("2006-01-02", r.ReleaseDate)
	if err != nil || date.Format("2006-01-02") != r.ReleaseDate {
		return fmt.Errorf("%w: release date must be YYYY-MM-DD", ErrInvalid)
	}
	for i := range r.Items {
		item := &r.Items[i]
		item.Description = strings.TrimSpace(item.Description)
		if item.Description == "" {
			return fmt.Errorf("%w: item %d needs a description", ErrInvalid, i+1)
		}
		switch item.ItemType {
		case "bug fix", "improvement", "new feature":
		default:
			return fmt.Errorf("%w: item %d has an invalid type", ErrInvalid, i+1)
		}
	}
	return nil
}

// naturalCompare compares digit runs by numeric value and other characters alphabetically.
func naturalCompare(a, b string) int {
	a, b = strings.ToLower(a), strings.ToLower(b)
	for len(a) > 0 && len(b) > 0 {
		ad, bd := a[0] >= '0' && a[0] <= '9', b[0] >= '0' && b[0] <= '9'
		if ad && bd {
			ai, bi := 0, 0
			for ai < len(a) && a[ai] >= '0' && a[ai] <= '9' {
				ai++
			}
			for bi < len(b) && b[bi] >= '0' && b[bi] <= '9' {
				bi++
			}
			an, bn := strings.TrimLeft(a[:ai], "0"), strings.TrimLeft(b[:bi], "0")
			if len(an) != len(bn) {
				return compareInt(len(an), len(bn))
			}
			if c := strings.Compare(an, bn); c != 0 {
				return c
			}
			a, b = a[ai:], b[bi:]
			continue
		}
		if a[0] != b[0] {
			return compareInt(int(a[0]), int(b[0]))
		}
		a, b = a[1:], b[1:]
	}
	return compareInt(len(a), len(b))
}

func compareInt(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func List(ctx context.Context, db *sql.DB) ([]Release, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT r.id, r.major_version, r.minor_version, r.release_notes,
		       to_char(r.release_date, 'YYYY-MM-DD'),
		       i.id, i.item_type, i.description, i.notes, i.pull_request, i.ticket_num
		FROM public.releases r LEFT JOIN public.release_items i ON i.release_id = r.id
		ORDER BY r.id, i.sort_order, i.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Release, 0)
	for rows.Next() {
		var r Release
		var id sql.NullInt64
		var typ, description, notes, pr, ticket sql.NullString
		if err := rows.Scan(&r.ID, &r.MajorVersion, &r.MinorVersion, &r.ReleaseNotes, &r.ReleaseDate,
			&id, &typ, &description, &notes, &pr, &ticket); err != nil {
			return nil, err
		}
		if len(result) == 0 || result[len(result)-1].ID != r.ID {
			r.Items = []Item{}
			result = append(result, r)
		}
		if id.Valid {
			last := &result[len(result)-1]
			last.Items = append(last.Items, Item{ID: id.Int64, ItemType: typ.String, Description: description.String,
				Notes: notes.String, PullRequest: pr.String, TicketNum: ticket.String})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(result, func(i, j int) bool {
		a, b := result[i], result[j]
		if c := naturalCompare(a.MajorVersion, b.MajorVersion); c != 0 {
			return c > 0
		}
		if c := naturalCompare(a.MinorVersion, b.MinorVersion); c != 0 {
			return c > 0
		}
		return a.ID > b.ID
	})
	return result, nil
}

func Save(ctx context.Context, db *sql.DB, id int64, r Release) (int64, error) {
	if err := validate(&r); err != nil {
		return 0, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if id == 0 {
		err = tx.QueryRowContext(ctx, `INSERT INTO public.releases
			(major_version, minor_version, release_notes, release_date) VALUES ($1, $2, $3, $4::date) RETURNING id`,
			r.MajorVersion, r.MinorVersion, r.ReleaseNotes, r.ReleaseDate).Scan(&id)
	} else {
		var updatedID int64
		err = tx.QueryRowContext(ctx, `UPDATE public.releases SET major_version=$2, minor_version=$3,
			release_notes=$4, release_date=$5::date, updated_at=NOW() WHERE id=$1 RETURNING id`,
			id, r.MajorVersion, r.MinorVersion, r.ReleaseNotes, r.ReleaseDate).Scan(&updatedID)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrNotFound
		}
		if err == nil {
			_, err = tx.ExecContext(ctx, `DELETE FROM public.release_items WHERE release_id=$1`, id)
		}
	}
	if err != nil {
		return 0, err
	}
	for i, item := range r.Items {
		_, err = tx.ExecContext(ctx, `INSERT INTO public.release_items
			(release_id, item_type, description, notes, pull_request, ticket_num, sort_order)
			VALUES ($1,$2,$3,$4,$5,$6,$7)`, id, item.ItemType, item.Description, item.Notes,
			item.PullRequest, item.TicketNum, i)
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
	res, err := db.ExecContext(ctx, `DELETE FROM public.releases WHERE id=$1`, id)
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
