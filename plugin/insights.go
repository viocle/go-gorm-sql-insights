package insights

import (
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
)

var (
	ErrTimedOut = errors.New("timed out")
)

// SQLInsights is a Gorm plugin that collects, aggregates, and stores SQL statistics
type SQLInsights struct {
	// _db is the gorm DB instance this plugin is registered to
	_db *gorm.DB

	// config is the configuration for this plugin
	config Config

	// InstanceAppID is the SQLInsightsApp ID for the the defined InstanceID
	instanceAppID uint

	// statistics table used in between storage intervals
	stats        map[statType]map[string][]*stat
	statsBuf     []*SQLInsightsHistory
	keyHashes    map[string]struct{}            // keyHash
	callerHashes map[string]map[string]struct{} // keyHash -> callerHash
	statsLock    sync.Mutex

	// stats channel to receive statistics from the Gorm callbacks
	statsChan chan *stat
	stopChan  chan chan struct{}
	stopped   bool

	statementMaps map[string]*sync.Map
}

type Config struct {
	// DB is the GORM DB instance to store the SQL statistics at
	// This Does not have to be the same DB instance as the one this plugin is being used as a plugin for it to monitor
	DB *gorm.DB

	// InstanceID is the ID/name of the application this plugin is being used in. Something unique to distinqguish this instance from other instances of the same or different applications
	// Example, "myapp:us-west-2a" if you have multiple instances of the same app running in different regions/availability zones in AWS and you want to have the ability to segregate the statistics by region/availability zone
	InstanceID string

	// if you want to collect the calling functions, specify the depth here
	// A value <1 means do not collect callers
	CollectCallerDepth int

	// MaxStatisticsBufferSize is the maximum number of statistics to buffer before before blocking
	MaxStatisticsBufferSize int

	// AutoPurgeAge is the age at which old statistics are automatically purged from the DB. A value of <=0 means do not automatically purge old statistics
	AutoPurgeAge time.Duration

	// CollectSystemResources specifies if system resource statistics (memory % used, CPU %) should be collected
	CollectSystemResources bool

	// DashboardConfig is the configuration for the dashboard user interface
	DashboardConfig *DashboardConfig

	// The maximum time to wait for the plugin to stop and flush any remaining statistics to the DB when being unregistered
	StopTimeLimit time.Duration

	// SkipAutomigration specifies if the plugin should skip the automigration of the statistics tables. If set to true, the tables should be manually migrated before using this plugin and after any plugin updates
	SkipAutomigration bool
}

// ApplyDefaults applies default values to the config if they are not set
func (c *Config) applyDefaults() {
	if c.CollectCallerDepth < 0 {
		c.CollectCallerDepth = 0
	}
	if c.MaxStatisticsBufferSize <= 0 {
		c.MaxStatisticsBufferSize = 100
	}
	if c.AutoPurgeAge < 0 {
		c.AutoPurgeAge = 0
	}

	// set up a default dashboard config if one is not provided
	if c.DashboardConfig == nil {
		c.DashboardConfig = &DashboardConfig{
			TimeLocation: time.UTC,
		}
	}

	// get hostname if InstanceID is empty
	c.InstanceID = strings.TrimSpace(c.InstanceID)
	if c.InstanceID == "" {
		if hostname, err := os.Hostname(); err == nil {
			c.InstanceID = hostname
		}
	}
}

// New creates a new Gorm SQLInsights plugin with specified config and starts the background collector and reporter.
// When the plugin is no longer needed, call Stop() to unregister this plugin and stop the background collector and reporter processes as well as store any existing statistics that haven't been reported yet
func New(config Config) *SQLInsights {
	// set default values if not specified
	config.applyDefaults()

	// create our new SQLInsights plugin instance
	ret := &SQLInsights{
		instanceAppID: 0,
		config:        config,
		stats:         make(map[statType]map[string][]*stat, 1),
		statsBuf:      make([]*SQLInsightsHistory, 0, 100),
		keyHashes:     make(map[string]struct{}, 1),
		callerHashes:  make(map[string]map[string]struct{}, 1),
		statsLock:     sync.Mutex{},
		statsChan:     make(chan *stat, config.MaxStatisticsBufferSize), // allow buffering of stats without blocking
		stopChan:      make(chan chan struct{}),
		statementMaps: map[string]*sync.Map{_statTypeQuery.String(): {}, _statTypeRaw.String(): {}},
	}

	if config.DB != nil {
		// perform automigration of our statistics tables
		ret.performAutoMigration()
	}

	// start background collector
	go ret.collector()

	// return our new SQLInsights
	return ret
}

// performAutoMigration performs the automigration of the statistics tables
func (s *SQLInsights) performAutoMigration() {
	if !s.config.SkipAutomigration {
		_ = s.StatDB().AutoMigrate(autoMigration()...)
	}

	// load/store our InstanceID/App name
	if s.config.InstanceID != "" {
		// store our InstanceID/App name if it currently does not exist
		appInstance := SQLInsightsApp{
			InstanceAppName: s.config.InstanceID,
		}
		if err := s.StatDB().Where("instance_app_name = ?", s.config.InstanceID).FirstOrCreate(&appInstance).Error; err == nil && appInstance.ID >= 0 {
			s.instanceAppID = appInstance.ID
		}
	}

	// load our known key hashes
	var keyHashes []SQLInsightsHash
	if err := s.StatDB().Find(&keyHashes).Error; err == nil {
		for _, keyHash := range keyHashes {
			s.keyHashes[keyHash.ID] = struct{}{}
		}
	}

	// load our known caller hashes
	if s.config.CollectCallerDepth > 0 {
		var callerHashes []SQLInsightsCallerHistory
		if err := s.StatDB().Find(&callerHashes).Error; err == nil {
			for _, callerHash := range callerHashes {
				if _, ok := s.callerHashes[callerHash.HashID]; !ok {
					s.callerHashes[callerHash.HashID] = make(map[string]struct{}, 1)
				}
				s.callerHashes[callerHash.HashID][callerHash.ID] = struct{}{}
			}
		}
	}
}

// IsStopped returns true if the plugin has been stopped
func (s *SQLInsights) IsStopped() bool {
	s.statsLock.Lock()
	defer s.statsLock.Unlock()
	return s.stopped
}

// Stop stops the plugin from collecting and reporting statistics. The allowedWaitTime parameter specifies how long to wait for the collector to finishing draining uncollected statistics before exiting
func (s *SQLInsights) Stop(allowedWaitTime time.Duration) error {
	if s.IsStopped() {
		// already stopped
		return nil
	}

	// signal to stop the collector, waiting for it to be received
	stoppedChan := make(chan struct{})
	s.stopChan <- stoppedChan

	// wait for the collector to stop
	<-stoppedChan

	// lock our stats table
	s.statsLock.Lock()
	defer s.statsLock.Unlock()

	// collect remaining statistics
	_ = s.unsafeDrainStatsChannel(allowedWaitTime)

	// flush any existing statistics to the DB
	s.unsafeReportStatistics(time.Now().UTC())

	// mark as stopped
	s.stopped = true

	return nil
}

// collector collects statistics from the stats channel and stores them in the stats table
func (s *SQLInsights) collector() {
	// aggregate and report our statistics every minute
	reportTicker := time.NewTicker(time.Minute)
	defer reportTicker.Stop()
	purgeInterval := time.Hour
	if s.config.AutoPurgeAge > purgeInterval*24 {
		// purge interval is not common, lets only attempt to purge once a day
		purgeInterval = purgeInterval * 24
	}
	purgeCheck := time.NewTicker(purgeInterval)
	defer purgeCheck.Stop()
	lastPurge := time.Time{}
	newStats := false
	for {
		select {
		case statValue := <-s.statsChan:
			// add this stat to our stats table
			s.statsLock.Lock()
			// store this statistic
			s.unsafeAddStat(statValue)
			newStats = true
			s.statsLock.Unlock()
		case <-reportTicker.C:
			// report the statistics if we have new values to report
			s.statsLock.Lock()
			if newStats {
				s.unsafeReportStatistics(time.Now().UTC())
				newStats = false
			}
			s.statsLock.Unlock()
		case <-purgeCheck.C:
			// purge old statistics
			lastPurge = s.purgeOldStatistics(lastPurge)
		case stopWait := <-s.stopChan:
			// stop the collector
			stopWait <- struct{}{}
			return
		}
	}
}

// purgeOldStatistics purges old statistics from the DB for this application instance ID
func (s *SQLInsights) purgeOldStatistics(lastPurge time.Time) time.Time {
	if s.config.AutoPurgeAge <= 0 {
		// not purging old statistics
		return lastPurge
	}
	if s.config.DB == nil {
		// no DB to purge old statistics from
		return lastPurge
	}
	if time.Since(lastPurge) < s.config.AutoPurgeAge {
		// not time to purge old statistics yet
		return lastPurge
	}
	go s.StatDB().Where("instance_id = ? AND created_at < ?", s.instanceAppID, time.Now().UTC().Add(-1*s.config.AutoPurgeAge)).Unscoped().Delete(SQLInsightsHistory{})
	return time.Now().UTC()
}

// unsafeReportStatistics aggregates all statistics in the stats table and stores them in the DB then clears the stats table
func (s *SQLInsights) unsafeReportStatistics(now time.Time) {
	if s.config.DB != nil {
		// collect system resources if enabled
		var resources systemResources
		if s.config.CollectSystemResources {
			resources = collectSystemResources()
		}

		// loop through our stats table and report each one
		keyHashIDs := make([]string, 0, 10)
		keyHashes := make(map[string]*SQLInsightsHash, 10)
		callerHistoryIDs := make([]string, 0, 10)
		callerHistories := make(map[string]*SQLInsightsCallerHistory, 10)
		for statType, statTypeMap := range s.stats {
			for keyHash, stats := range statTypeMap {
				if keyHash != "" && len(stats) > 0 {
					// store the key hash in the DB if it currently does not exist
					if _, ok := s.keyHashes[keyHash]; !ok {
						s.keyHashes[keyHash] = struct{}{}
						keyHashIDs = append(keyHashIDs, keyHash)
						keyHashes[keyHash] = &SQLInsightsHash{
							ID:        keyHash,
							CreatedAt: now,
							Statement: stats[0].Key,
							NumVars:   stats[0].NumVars,
						}
					}

					// build the stat and caller history (if enabled)
					if statHistory, callerHistory := s.buildStatHistory(now, keyHash, statType, stats); statHistory != nil {
						// add system resources if enabled
						if s.config.CollectSystemResources {
							statHistory.CPU = resources.CPUPercentage
							statHistory.Mem = resources.MemoryPercentage
						}

						// add to statsBuf for bulk insert
						s.statsBuf = append(s.statsBuf, statHistory)

						if len(callerHistory) > 0 {
							// store the caller history in the DB if they currently do not exist
							for _, callerHistoryValue := range callerHistory {
								if _, ok := s.callerHashes[keyHash]; !ok {
									s.callerHashes[keyHash] = make(map[string]struct{}, 1)
								}
								if _, ok := s.callerHashes[keyHash][callerHistoryValue.ID]; !ok {
									// we have not seen this caller hash before, so store it and log it in our local hash table
									s.callerHashes[keyHash][callerHistoryValue.ID] = struct{}{}
									callerHistoryIDs = append(callerHistoryIDs, keyHash+callerHistoryValue.ID)
									callerHistories[keyHash+callerHistoryValue.ID] = callerHistoryValue
								}
							}
						}
					}
				}
			}
		}

		if len(keyHashes) > 0 {
			// check if we have these SQLInsightsHash values in the DB and insert if we don't
			// query for existing key hashes using our keyHashIDs list
			var existingKeyHashes []string
			_ = s.StatDB().Model(&SQLInsightsHash{}).Select("id").Where("id IN ?", keyHashIDs).Scan(&existingKeyHashes)
			// removing existing values from keyHashes where we already have them in the DB
			for _, existingKeyHash := range existingKeyHashes {
				delete(keyHashes, existingKeyHash)
			}
			if len(keyHashes) > 0 {
				// insert our new key hashes
				toInsert := make([]*SQLInsightsHash, 0, len(keyHashes))
				for _, keyHash := range keyHashes {
					toInsert = append(toInsert, keyHash)
				}
				_ = s.StatDB().Create(toInsert)
			}
		}

		if len(callerHistories) > 0 {
			// check if we have these SQLInsightsCallerHistory values in the DB and insert if we don't
			// query for existing caller hashes using our callerHistoryIDs list
			var existingCallerHashes []string
			_ = s.StatDB().Model(&SQLInsightsCallerHistory{}).Select("CONCAT(id, hash_id) AS id").Where("id IN ?", callerHistoryIDs).Scan(&existingCallerHashes)
			// removing existing values from callerHistories where we already have them in the DB
			for _, existingCallerHash := range existingCallerHashes {
				delete(callerHistories, existingCallerHash)
			}
			if len(callerHistories) > 0 {
				// insert our new caller hashes
				toInsert := make([]*SQLInsightsCallerHistory, 0, len(callerHistories))
				for _, callerHistory := range callerHistories {
					toInsert = append(toInsert, callerHistory)
				}
				_ = s.StatDB().Create(toInsert)
			}
		}

		if len(s.statsBuf) == 0 {
			// no stats to report
			return
		}

		// store our stats in the DB
		_ = s.StatDB().Create(s.statsBuf)

		// clear our statsBuf but keep the capacity
		clear(s.statsBuf)
		s.statsBuf = s.statsBuf[:0]

		// clear our stats table, leaving our map types and their hashes allocated
		for _, statTypeMap := range s.stats {
			for hashKey := range statTypeMap {
				clear(statTypeMap[hashKey])
				statTypeMap[hashKey] = statTypeMap[hashKey][:0]
			}
		}
	}
}

// unsafeAddStat stores the statistic in the stats table. It is not thread safe and assumes statsLock is already locked
func (s *SQLInsights) unsafeAddStat(statValue *stat) {
	if statValue.KeyHash == "" {
		return
	}
	// store the statistic in the stats table
	if _, ok := s.stats[statValue.Type]; !ok {
		s.stats[statValue.Type] = make(map[string][]*stat, 10)
	}
	if _, ok := s.stats[statValue.Type][statValue.KeyHash]; !ok {
		s.stats[statValue.Type][statValue.KeyHash] = make([]*stat, 0, 100)
	}
	s.stats[statValue.Type][statValue.KeyHash] = append(s.stats[statValue.Type][statValue.KeyHash], statValue)
}

// DrainStatsChannel drains the stats channel and stores the statistics in the stats table. It will wait for the specified timeOut duration before returning an error if the channel is not empty
func (s *SQLInsights) DrainStatsChannel(timeOut time.Duration) error {
	// lock our stats table
	s.statsLock.Lock()
	defer s.statsLock.Unlock()

	// collect remaining statistics
	return s.unsafeDrainStatsChannel(timeOut)
}

// unsafeDrainStatsChannel drains the stats channel and stores the statistics in the stats table. It is not thread safe and assumes the statsLock is already locked
func (s *SQLInsights) unsafeDrainStatsChannel(timeOut time.Duration) error {
	t := time.NewTimer(timeOut)
	defer t.Stop()
	for {
		select {
		case statValue := <-s.statsChan:
			// add this stat
			s.unsafeAddStat(statValue)
		case <-t.C:
			// timeout, exit
			return ErrTimedOut
		default:
			// no more stats in the buffered channel, exit
			return nil
		}
	}
}

// insightsAddStat adds a statistic to be collected by the background collector
func (s *SQLInsights) insightsAddStat(statValue *stat) {
	if statValue == nil || statValue.Key == "" {
		return
	}

	// create hash of the key (parameterized SQL statement)
	statValue.KeyHash = hash(statValue.Key)

	// get hash of our callers if we have any and are tracking this
	if len(statValue.Callers) > 0 && s.config.CollectCallerDepth > 0 {
		// we have one ore more callers, serialize and hash
		statValue.CallerJSON, _ = json.Marshal(statValue.Callers)
		if len(statValue.CallerJSON) > 0 {
			statValue.CallerHash = hashBytes(statValue.CallerJSON)
		}
	}

	// send to the stats channel, wait if buffer is full
	s.statsChan <- statValue
}

// hash returns the MD5 hash of the input string
func hash(s string) string {
	// file deepcode ignore InsecureHash: not used for cryptographic purposes
	return hashBytes([]byte(s))
}

// hashBytes returns the MD5 hash of the input byte slice
func hashBytes(b []byte) string {
	// file deepcode ignore InsecureHash: not used for cryptographic purposes
	return fmt.Sprintf("%x", md5.Sum(b))
}
