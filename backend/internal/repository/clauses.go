package repository

import "gorm.io/gorm/clause"

// clauseDoNothing returns an ON CONFLICT DO NOTHING clause for idempotent upserts.
func clauseDoNothing() clause.Expression {
	return clause.OnConflict{DoNothing: true}
}
