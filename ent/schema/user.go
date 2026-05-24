package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// User holds the schema definition for the User entity.
//
// A User is a platform tenant — somebody who signs up, logs into the
// merchant-facing dashboard, and owns one or more Products. Products carry
// the EasyPay API credentials (pid/pkey). Money received across a user's
// products flows into one Settlement record per user, and is withdrawn
// against that single balance.
type User struct {
	ent.Schema
}

func (User) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.String("email").
			NotEmpty().
			Unique().
			Comment("Login identifier"),
		field.String("password_hash").
			NotEmpty().
			Sensitive(),
		field.String("name").
			NotEmpty().
			Comment("Display name shown in admin/user dashboards"),
		field.Float("fee_rate").
			SchemaType(map[string]string{
				dialect.Postgres: "decimal(10,4)",
			}).
			Default(0.006).
			Comment("Platform fee rate applied to a user's orders unless overridden by product.fee_rate"),
		field.Enum("status").
			Values("active", "disabled").
			Default("active"),
		field.Time("created_at").
			Immutable().
			Default(time.Now),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("products", Product.Type),
		edge.To("orders", Order.Type),
		edge.To("settlements", Settlement.Type),
		edge.To("withdraws", Withdraw.Type),
		edge.To("refunds", Refund.Type),
	}
}

func (User) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("email").Unique(),
	}
}

func (User) Annotations() []schema.Annotation {
	return nil
}
