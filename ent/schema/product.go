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

// Product holds the schema definition for the Product entity.
//
// A Product belongs to one User and carries the EasyPay API credentials
// (pid + pkey) used by third parties to call /mapi.php. Each product gets
// its own pair so a user can isolate their callsites (e.g. "blog donations"
// vs "course purchases"). Fees default to the user's fee_rate; setting
// fee_rate on a product overrides it for orders created under that product.
type Product struct {
	ent.Schema
}

func (Product) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("user_id", uuid.UUID{}),
		field.Int("pid").
			Unique().
			Immutable().
			Comment("Public merchant id used in EasyPay protocol"),
		field.String("pkey").
			NotEmpty().
			Sensitive().
			Comment("API signing key — MD5 secret for the EasyPay protocol"),
		field.String("name").
			NotEmpty().
			Comment("Product/project display name"),
		field.String("description").
			Optional().
			Default(""),
		field.String("notify_url").
			Optional().
			Default("").
			Comment("Default merchant async-notify URL for this product"),
		field.String("return_url").
			Optional().
			Default("").
			Comment("Default merchant sync return URL for this product"),
		field.Float("fee_rate").
			SchemaType(map[string]string{
				dialect.Postgres: "decimal(10,4)",
			}).
			Optional().
			Nillable().
			Comment("Optional override of user.fee_rate"),
		field.Enum("status").
			Values("active", "disabled").
			Default("active"),
		// Legacy EasyPay protocol options carried over from the old Merchant entity.
		field.Int("keytype").
			Default(0).
			Comment("0 = MD5, 1 = RSA (forces RSA sign_type)"),
		field.Text("public_key").
			Optional().
			Default("").
			Comment("Product RSA public key (PEM or base64) when keytype=1"),
		field.Bool("refund_enabled").
			Default(false),
		field.Bool("transfer_enabled").
			Default(false),
		field.Int("mode").
			Default(0).
			Comment("0 = standard, 1 = surcharge mode (placeholder)"),
		field.Time("created_at").
			Immutable().
			Default(time.Now),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

func (Product) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("products").
			Field("user_id").
			Unique().
			Required(),
		edge.To("orders", Order.Type),
	}
}

func (Product) Annotations() []schema.Annotation {
	return nil
}
