package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// Admin holds the schema definition for the Admin entity.
type Admin struct {
	ent.Schema
}

func (Admin) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.String("username").
			NotEmpty().
			Unique(),
		field.String("password_hash").
			NotEmpty(),
		field.Time("created_at").
			Immutable().
			Default(time.Now),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

func (Admin) Edges() []ent.Edge {
	return nil
}

func (Admin) Annotations() []schema.Annotation {
	return nil
}
