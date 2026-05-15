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

// Order holds the schema definition for the Order entity.
type Order struct {
	ent.Schema
}

func (Order) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.String("order_no").
			NotEmpty().
			Unique(),
		field.UUID("merchant_id", uuid.UUID{}),
		field.Enum("type").
			Values("alipay", "wxpay"),
		field.Float("amount").
			SchemaType(map[string]string{
				dialect.Postgres: "decimal(20,2)",
			}),
		field.Float("fee_official").
			SchemaType(map[string]string{
				dialect.Postgres: "decimal(20,2)",
			}).
			Default(0),
		field.Float("fee_platform").
			SchemaType(map[string]string{
				dialect.Postgres: "decimal(20,2)",
			}).
			Default(0),
		field.Float("net_amount").
			SchemaType(map[string]string{
				dialect.Postgres: "decimal(20,2)",
			}).
			Default(0),
		field.String("trade_no").
			Optional().
			Default(""),
		field.Enum("status").
			Values("PENDING", "PAID", "SETTLED", "EXPIRED", "CANCELLED").
			Default("PENDING"),
		field.String("notify_url").
			NotEmpty(),
		field.String("provider_snapshot").
			Optional().
			Default(""),
		field.Time("paid_at").
			Optional().
			Nillable(),
		field.Time("created_at").
			Immutable().
			Default(time.Now),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

func (Order) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("merchant", Merchant.Type).
			Ref("orders").
			Field("merchant_id").
			Unique().
			Required(),
	}
}

func (Order) Annotations() []schema.Annotation {
	return nil
}
