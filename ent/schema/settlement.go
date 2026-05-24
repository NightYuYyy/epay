package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// Settlement holds the schema definition for the Settlement entity.
//
// One row per User. Tracks net balance available for withdrawal.
type Settlement struct {
	ent.Schema
}

func (Settlement) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("user_id", uuid.UUID{}).
			Unique(),
		field.Float("balance").
			SchemaType(map[string]string{
				dialect.Postgres: "decimal(20,2)",
			}).
			Default(0),
		field.Float("frozen").
			SchemaType(map[string]string{
				dialect.Postgres: "decimal(20,2)",
			}).
			Default(0),
		field.Float("total_income").
			SchemaType(map[string]string{
				dialect.Postgres: "decimal(20,2)",
			}).
			Default(0),
		field.Float("total_withdrawn").
			SchemaType(map[string]string{
				dialect.Postgres: "decimal(20,2)",
			}).
			Default(0),
		field.Time("created_at").
			Immutable().
			Default(time.Now),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

func (Settlement) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("settlements").
			Field("user_id").
			Unique().
			Required(),
	}
}

func (Settlement) Annotations() []schema.Annotation {
	return nil
}
