package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// Refund records a refund request and its lifecycle.
// Aligned with rainbow-epay `pre_refundorder` table.
type Refund struct {
	ent.Schema
}

func (Refund) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.String("refund_no").
			NotEmpty().
			Unique().
			Comment("Platform-generated refund number"),
		field.String("out_refund_no").
			Optional().
			Default("").
			Comment("Caller-supplied refund number for idempotency"),
		field.String("trade_no").
			NotEmpty(),
		field.UUID("user_id", uuid.UUID{}),
		field.Float("money").
			SchemaType(map[string]string{
				dialect.Postgres: "decimal(20,2)",
			}),
		field.Float("reduce_money").
			SchemaType(map[string]string{
				dialect.Postgres: "decimal(20,2)",
			}).
			Default(0).
			Comment("Amount deducted from the user's balance (may differ from refund money)"),
		field.Enum("status").
			Values("PENDING", "SUCCESS", "FAILED").
			Default("PENDING"),
		field.String("message").
			Optional().
			Default(""),
		field.Time("created_at").
			Immutable().
			Default(time.Now),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
		field.Time("finished_at").
			Optional().
			Nillable(),
	}
}

func (Refund) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("refunds").
			Field("user_id").
			Unique().
			Required(),
	}
}

func (Refund) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "out_refund_no").
			Unique().
			Annotations(entsql.IndexWhere("out_refund_no <> ''")),
	}
}

func (Refund) Annotations() []schema.Annotation {
	return nil
}
