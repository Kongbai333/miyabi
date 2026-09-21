package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

// ViewedMovie records that a JavDB movie detail page has been opened, so the
// discover grid can dim titles the user already looked at. The mixin's
// created_at doubles as the first-viewed time.
type ViewedMovie struct {
	ent.Schema
}

func (ViewedMovie) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (ViewedMovie) Fields() []ent.Field {
	return []ent.Field{
		field.String("movie_id").
			NotEmpty().
			Unique(),
	}
}
