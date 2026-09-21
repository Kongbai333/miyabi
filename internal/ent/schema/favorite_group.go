package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// FavoriteGroup is a user-created bucket for starred movies. Groups are
// independent of the local scan, so renaming one never touches the index.
type FavoriteGroup struct {
	ent.Schema
}

func (FavoriteGroup) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (FavoriteGroup) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			NotEmpty().
			Unique(),
	}
}

func (FavoriteGroup) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("favorites", Favorite.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}
