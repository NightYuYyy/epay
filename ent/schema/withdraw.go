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

// Withdraw holds the schema definition for the Withdraw entity.
type Withdraw struct {
	ent.Schema
}

func (Withdraw) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("user_id", uuid.UUID{}),
		field.Float("amount").
			SchemaType(map[string]string{
				dialect.Postgres: "decimal(20,2)",
			}),
		field.String("account_info").
			NotEmpty().
			SchemaType(map[string]string{
				dialect.Postgres: "text",
			}),
		field.Enum("status").
			Values("PENDING", "APPROVED", "PAID", "REJECTED").
			Default("PENDING"),
		field.String("remark").
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

func (Withdraw) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("withdraws").
			Field("user_id").
			Unique().
			Required(),
	}
}

func (Withdraw) Annotations() []schema.Annotation {
	return nil
}
