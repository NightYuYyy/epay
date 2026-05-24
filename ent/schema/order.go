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

// Order holds the schema definition for the Order entity.
//
// `order_no` is the merchant-supplied out_trade_no in EasyPay terminology.
// Every order belongs to one Product, and (denormalized) to that product's
// User so user-scope queries don't need a join.
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
		field.UUID("product_id", uuid.UUID{}),
		field.UUID("user_id", uuid.UUID{}).
			Comment("Denormalized from product.user_id for fast user-scope queries"),
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
		// EasyPay protocol fields (aligned with rainbow-epay pre_order)
		field.String("api_trade_no").
			Optional().
			Default(""),
		field.String("buyer").
			Optional().
			Default(""),
		field.String("param").
			Optional().
			Default(""),
		field.String("name").
			Optional().
			Default(""),
		field.String("clientip").
			Optional().
			Default(""),
		field.String("return_url").
			Optional().
			Default(""),
		field.String("device").
			Optional().
			Default("pc"),
		field.String("method").
			Optional().
			Default(""),
		field.String("sub_openid").
			Optional().
			Default(""),
		field.String("sub_appid").
			Optional().
			Default(""),
		field.String("auth_code").
			Optional().
			Default(""),
		field.Float("refund_money").
			SchemaType(map[string]string{
				dialect.Postgres: "decimal(20,2)",
			}).
			Default(0),
		field.Int("version").
			Default(0).
			Comment("0 = MD5 standard interface, 1 = RSA s=path API_INIT mode"),
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
		edge.From("product", Product.Type).
			Ref("orders").
			Field("product_id").
			Unique().
			Required(),
		edge.From("user", User.Type).
			Ref("orders").
			Field("user_id").
			Unique().
			Required(),
	}
}

func (Order) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("trade_no").
			Unique().
			Annotations(entsql.IndexWhere("trade_no <> ''")),
	}
}

func (Order) Annotations() []schema.Annotation {
	return nil
}
