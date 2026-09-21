package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Favorite marks a library movie as starred under one group. A movie may sit
// in several groups, so the pair is unique rather than the movie alone.
type Favorite struct {
	ent.Schema
}

func (Favorite) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (Favorite) Fields() []ent.Field {
	return []ent.Field{
		field.Int("movie_id").
			Positive(),
		field.Int("group_id").
			Positive(),
	}
}

func (Favorite) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("movie", Movie.Type).
			Ref("favorites").
			Field("movie_id").
			Unique().Required(),
		edge.From("group", FavoriteGroup.Type).
			Ref("favorites").
			Field("group_id").
			Unique().Required(),
	}
}

func (Favorite) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("movie_id", "group_id").Unique(),
		index.Fields("group_id"),
	}
}
