package store

import (
	"errors"

	"github.com/disgoorg/snowflake/v2"
	"github.com/jackc/pgx/v5"
)

// ErrNotFound is returned when a row does not exist (or belongs to another
// guild).
var ErrNotFound = errors.New("not found")

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// Discord IDs are stored as BIGINT and converted explicitly at the boundary.

func nullableID(id *snowflake.ID) *int64 {
	if id == nil || *id == 0 {
		return nil
	}
	v := int64(*id)
	return &v
}

func idFromNullable(v *int64) *snowflake.ID {
	if v == nil {
		return nil
	}
	id := snowflake.ID(*v)
	return &id
}

func toInt64s(ids []snowflake.ID) []int64 {
	out := make([]int64, len(ids))
	for i, id := range ids {
		out[i] = int64(id)
	}
	return out
}

func fromInt64s(v []int64) []snowflake.ID {
	out := make([]snowflake.ID, len(v))
	for i, id := range v {
		out[i] = snowflake.ID(id)
	}
	return out
}
