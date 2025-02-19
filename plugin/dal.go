package insights

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// SQLInsightsHash defines a hash of a SQL statement and the first time it was seen
type SQLInsightsHash struct {
	ID        string    `gorm:"size:32;primaryKey"` // key hash
	CreatedAt time.Time `gorm:"type:datetime(6)"`   // created/first seen
	Statement string    `gorm:"size:4096"`          // SQL statement our hash is based on
	NumVars   int       ``                          // number of variables in the SQL statement
}

// SQLInsightsApp defines an application/instance so we can segregate statistics by different applications or instances. Ex. API instances running in different regions or Lambda functions
type SQLInsightsApp struct {
	ID              uint   `gorm:"primaryKey;autoIncrement"` // auto incrementing ID
	InstanceAppName string `gorm:"size:191;index"`           // application/Instance ID/name
}

// SQLInsightsHistory defines a historical record of a specific SQL statement at the specified time for the specified instance
type SQLInsightsHistory struct {
	ID         uint      `gorm:"primaryKey;autoIncrement"` // auto incrementing ID
	InstanceID uint      `gorm:"index"`                    // SQLInsightsApp ID
	CreatedAt  time.Time `gorm:"type:datetime(6)"`         // History aggregation
	HashID     string    `gorm:"size:32;index"`            // hash ID
	Type       statType  `gorm:"size:12;index"`            // stat type
	Errors     int       ``                                // number of errors
	CPU        float64   `gorm:"type:decimal(3,2)"`        // CPU percentage (0.00-1.00)
	Mem        float64   `gorm:"type:decimal(3,2)"`        // memory percentage (0.00-1.00)
	Count      int       ``                                // number of executions
	RowsMin    int64     `gorm:"type:bigint"`              // minimum number of rows affected/returned
	RowsMax    int64     `gorm:"type:bigint"`              // maximum number of rows affected/returned
	RowsAvg    int64     `gorm:"type:bigint"`              // average/mean number of rows affected/returned
	RowsSum    int64     `gorm:"type:bigint"`              // total number of rows affected/returned
	RowsMed    int64     `gorm:"type:bigint"`              // median number of rows affected/returned
	TookMin    float64   `gorm:"type:decimal(14,6)"`       // minimum execution duration in fractional milliseconds
	TookMax    float64   `gorm:"type:decimal(14,6)"`       // maximum execution duration in fractional milliseconds
	TookAvg    float64   `gorm:"type:decimal(14,6)"`       // average/mean execution duration in fractional milliseconds
	TookMed    float64   `gorm:"type:decimal(14,6)"`       // median execution duration in fractional milliseconds
	TookSum    float64   `gorm:"type:decimal(14,6)"`       // total execution duration in fractional milliseconds
}

// SQLInsightsCallerHistory defines a historcal record of a specified SQL statement and the caller information when first seen
type SQLInsightsCallerHistory struct {
	ID        string    `gorm:"primaryKey;size:32"`       // caller hash
	CreatedAt time.Time `gorm:"type:datetime(6)"`         // created/first seen
	HashID    string    `gorm:"primaryKey;size:32;index"` // hash ID
	Value     []byte    `gorm:"type:LONGBLOB"`            // caller value
}

// SetValue serializes the callers as a JSON string and stores result in Value
func (s *SQLInsightsCallerHistory) SetValue(callers []*callerInfo) {
	// serialize callers as JSON string and store result in Value
	if len(callers) > 0 {
		if b, err := json.Marshal(callers); err == nil {
			s.Value = b
		}
	}
}

// SetJSON sets the Value as a JSON string
func (s *SQLInsightsCallerHistory) SetJSON(b []byte) {
	s.Value = b
}

// GetValue deserializes the Value as a JSON string and returns the callers
func (s *SQLInsightsCallerHistory) GetValue() []*callerInfo {
	// deserialize Value as a JSON string and return the callers
	var callers []*callerInfo
	if len(s.Value) > 0 {
		if err := json.Unmarshal(s.Value, &callers); err != nil {
			return nil
		}
	}
	return callers
}

// autoMigration returns the list of tables to auto migrate
func autoMigration() []interface{} {
	return []interface{}{
		&SQLInsightsApp{},
		&SQLInsightsHash{},
		&SQLInsightsHistory{},
		&SQLInsightsCallerHistory{},
	}
}

// StatDB returns the DB instance used by the SQLInsights to store/query statistics, skipping hooks, just in case the same DB instance being monitored is used to store the statistics
func (s *SQLInsights) StatDB() *gorm.DB {
	return s.config.DB.Session(&gorm.Session{NewDB: true, SkipHooks: true})
}
