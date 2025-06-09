package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// CronJobLock holds the schema definition for the CronJobLock entity.
type CronJobLock struct {
	ent.Schema
}

// Fields of the CronJobLock.
func (CronJobLock) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").
			Positive(),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
		field.String("job_name").
			NotEmpty(),
		field.String("job_identifier").
			NotEmpty(),
		field.String("worker").
			NotEmpty(),
		field.String("status").
			NotEmpty(),
	}
}

// Edges of the CronJobLock.
func (CronJobLock) Edges() []ent.Edge {
	return nil
}

// Indexes of the CronJobLock.
func (CronJobLock) Indexes() []ent.Index {
	return []ent.Index{
		// Unique constraint on job_name + job_identifier combination
		index.Fields("job_name", "job_identifier").
			Unique(),
	}
}
