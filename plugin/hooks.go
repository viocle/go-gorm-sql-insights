package insights

import (
	"time"
	"unsafe"

	"gorm.io/gorm"
)

// getTimeTaken calculates the time taken for a query to execute, returning the duration in fractional milliseconds, the current time, and any error that occurred
func (s *SQLInsights) getTimeTaken(ctxMapKey string, db *gorm.DB) (float64, time.Time, error) {
	// set the current time and retrieve the "in time" from our statement map for comparison
	now := time.Now().UTC()

	// calculate the execution duration in fractional milliseconds
	var took float64
	if inTimeA, ok := s.statementMaps[ctxMapKey].LoadAndDelete(*(*uint64)(unsafe.Pointer(db.Statement))); ok {
		if inTime, ok := inTimeA.(time.Time); ok {
			took = float64(int64(float64(now.Sub(inTime).Nanoseconds())/1e9)) / 1000 // fractional milliseconds
			// return our execution duration
			return took, now, nil
		}
	}

	return 0, now, gorm.ErrInvalidData
}

// insightsBefore is a generic callback that is called before a query is executed to inject the current time into the context
func (s *SQLInsights) insightsBefore(sType statType) func(*gorm.DB) {
	ctxMapKey := sType.String()
	return func(db *gorm.DB) {
		if db == nil || db.Statement == nil || db.Config == nil || db.Config.DryRun {
			// dont track with a nil db, statement, and/or missing config or if this is a dry run
			return
		}

		// store our current time in our statement map using the statement pointer address as the key as this should be unique for each statement, at least in the context of a single request
		s.statementMaps[ctxMapKey].Store(*(*uint64)(unsafe.Pointer(db.Statement)), time.Now().UTC())

	}
}

// insightsAfter is a generic callback that is called after a query is executed to collect the execution details
func (s *SQLInsights) insightsAfter(sType statType) func(*gorm.DB) {
	ctxMapKey := sType.String()
	return func(db *gorm.DB) {
		if db == nil || db.Statement == nil || db.Config == nil || db.Config.DryRun {
			// dont track with a nil db, statement, and/or missing config or if this is a dry run
			return
		}
		if took, now, err := s.getTimeTaken(ctxMapKey, db); err == nil {
			// report our non parametrized SQL statement with execution details
			v := &stat{
				TimeStamp: now,
				Type:      sType,
				Key:       db.Statement.SQL.String(),
				NumVars:   len(db.Statement.Vars),
				Took:      took,
				Rows:      db.RowsAffected,
				Error:     db.Error != nil,
				Callers:   getCallers(s.config.CollectCallerDepth),
			}
			go s.insightsAddStat(v)
		}
	}
}
