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

// Merchant holds the schema definition for the Merchant entity.
type Merchant struct {
	ent.Schema
}

func (Merchant) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.Int("pid").
			Unique().
			Immutable(),
		field.String("pkey").
			NotEmpty(),
		field.String("name").
			NotEmpty(),
		field.Float("fee_rate").
			SchemaType(map[string]string{
				dialect.Postgres: "decimal(10,4)",
			}).
			Default(1.0),
		field.Enum("status").
			Values("active", "disabled").
			Default("active"),
		field.String("notify_url").
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

func (Merchant) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("orders", Order.Type),
		edge.To("settlements", Settlement.Type),
		edge.To("withdraws", Withdraw.Type),
	}
}

func (Merchant) Annotations() []schema.Annotation {
	return nil
}
