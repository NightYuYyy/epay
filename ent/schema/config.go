package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// PlatformConfig holds the schema definition for the PlatformConfig entity.
type PlatformConfig struct {
	ent.Schema
}

func (PlatformConfig) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.String("key").
			NotEmpty().
			Unique(),
		field.String("value").
			Optional().
			Default("").
			SchemaType(map[string]string{
				dialect.Postgres: "text",
			}).
			Comment("Empty string is allowed — it means the configuration entry is intentionally unset."),
		field.String("description").
			Optional().
			Default(""),
		field.Time("created_at").
			Immutable().
			Default(time.Now),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

func (PlatformConfig) Edges() []ent.Edge {
	return nil
}

func (PlatformConfig) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "configs"},
	}
}
