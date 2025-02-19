package insights

import (
	_ "runtime/pprof"

	"gorm.io/gorm"
)

const (
	_eventBeforeQuery = "gorm-SQLInsights:before_query"
	_eventAfterQuery  = "gorm-SQLInsights:after_query"
	_eventBeforeRaw   = "gorm-SQLInsights:before_raw"
	_eventAfterRaw    = "gorm-SQLInsights:after_raw"
)

var (
	// Register the plugin
	_ gorm.Plugin = &SQLInsights{}
)

// Name returns the Name of this Gorm plugin
func (s *SQLInsights) Name() string {
	return "SQLInsights"
}

// Initialize initializes the plugin with the specified Gorm DB instance
func (s *SQLInsights) Initialize(db *gorm.DB) (err error) {
	if db == nil {
		return gorm.ErrInvalidDB
	}

	// store the DB instance we've initialized with
	s._db = db

	// Register our callbacks in the provided gorm DB instance
	for _, e := range []error{
		db.Callback().Query().Before("gorm:query").Register(_eventBeforeQuery, s.insightsBefore(_statTypeQuery)),
		db.Callback().Query().After("gorm:query").Register(_eventAfterQuery, s.insightsAfter(_statTypeQuery)),
		db.Callback().Raw().Before("gorm:raw").Register(_eventBeforeRaw, s.insightsBefore(_statTypeRaw)),
		db.Callback().Raw().After("gorm:raw").Register(_eventAfterRaw, s.insightsAfter(_statTypeRaw)),
	} {
		if e != nil {
			return e
		}
	}
	return nil
}

// unregister this plugin from the specified Gorm DB instance
func (s *SQLInsights) unregister() (err error) {
	if s._db == nil {
		return gorm.ErrInvalidDB
	}

	// stop the plugin and flush all data to the DB
	s.Stop(s.config.StopTimeLimit)

	// Unregister our callbacks from the stored gorm DB instance we received during initialization
	for _, e := range []error{
		s._db.Callback().Query().Remove(_eventBeforeQuery),
		s._db.Callback().Query().Remove(_eventAfterQuery),
		s._db.Callback().Raw().Remove(_eventBeforeRaw),
		s._db.Callback().Raw().Remove(_eventAfterRaw),
	} {
		if e != nil {
			return e
		}
	}
	return
}
