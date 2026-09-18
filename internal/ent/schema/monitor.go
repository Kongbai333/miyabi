package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Monitor watches a JavDB movie without a magnet and submits the first
// magnet to 115 once it appears.
type Monitor struct {
	ent.Schema
}

func (Monitor) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (Monitor) Fields() []ent.Field {
	return []ent.Field{
		field.String("movie_id").
			NotEmpty().
			Unique(),
		field.String("code").
			NotEmpty(),
		field.String("title").
			Default(""),
		field.String("cover").
			Default(""),
		field.String("release_date").
			Default(""),
		field.Enum("status").
			Values("waiting", "added", "stale").
			Default("waiting"),
		field.String("hash").
			Default(""),
		field.Int("task_id").
			Optional().
			Nillable(),
		field.Time("next_check_at").
			Optional().
			Nillable(),
		field.Time("last_checked_at").
			Optional().
			Nillable(),
		field.Int("checks").
			NonNegative().
			Default(0),
		field.String("error").
			Optional().
			Nillable(),
	}
}

func (Monitor) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status", "next_check_at"),
	}
}
