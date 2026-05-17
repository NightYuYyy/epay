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

// Refund records a merchant refund request and its lifecycle.
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
			Comment("Merchant-supplied refund number for idempotency"),
		field.String("trade_no").
			NotEmpty(),
		field.UUID("merchant_id", uuid.UUID{}),
		field.Float("money").
			SchemaType(map[string]string{
				dialect.Postgres: "decimal(20,2)",
			}),
		field.Float("reduce_money").
			SchemaType(map[string]string{
				dialect.Postgres: "decimal(20,2)",
			}).
			Default(0).
			Comment("Amount deducted from merchant balance (may differ from refund money)"),
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
		edge.From("merchant", Merchant.Type).
			Ref("refunds").
			Field("merchant_id").
			Unique().
			Required(),
	}
}

func (Refund) Annotations() []schema.Annotation {
	return nil
}
