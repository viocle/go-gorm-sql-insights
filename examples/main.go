package main

import (
	"time"

	insights "github.com/viocle/go-gorm-sql-insights/plugin"
	"gorm.io/gorm"

	// mysql driver for gorm
	"gorm.io/driver/mysql"
)

func main() {
	// initialize the database
	gormDialector := mysql.New(mysql.Config{
		DSN:                       "user:pass@127.0.0.1:3306/mydb?charset=utf8mb4&parseTime=True", // data source name
		DefaultStringSize:         256,                                                            // default size for string fields
		DontSupportRenameIndex:    true,                                                           // drop & create when rename index, rename index not supported before MySQL 5.7, MariaDB
		DontSupportRenameColumn:   true,                                                           // `change` when rename column, rename column not supported before MySQL 8, MariaDB
		SkipInitializeWithVersion: false,                                                          // auto configure based on currently MySQL version
	})
	db, err := gorm.Open(gormDialector, &gorm.Config{})
	if err != nil {
		panic("Error while initializing database: " + err.Error())
	}
	addPlugins(db)
}

func addPlugins(db *gorm.DB) {
	// load the US/Pacific time location so times on the dashboard are processed in this time zone
	timeLocation, err := time.LoadLocation("US/Pacific")
	if err != nil || timeLocation == nil {
		// something went wrong, default to UTC
		timeLocation = time.UTC
	}

	// call Use and pass in your new SQLInsights instance with our configuration
	// you may want to store a copy of the SQLInsights object returned by new if you want to interact with the plugin instance, like calling Stop to stop recording
	db.Use(insights.New(insights.Config{
		DB:                     db,                    // pass gorm DB instance to use for statistics storage. This could be another DB instance than the one being monitored if you wanted to store your statistics tables somewhere else
		InstanceID:             "my-test-app:server1", // name this instance with a semi unique ID
		CollectCallerDepth:     5,                     // if we want to collect details about where the SQL query was performed, enter how far we want to look up the call chain
		AutoPurgeAge:           time.Hour * 24 * 7,    // automatically purge data older than 7 days
		CollectSystemResources: true,                  // periodically collect the CPU and memory usage
		StopTimeLimit:          time.Second * 5,       // default length of time to wait when stopping the plugin when the plugin is being unregistered
		SkipAutomigration:      false,                 // if you want to skip the automigration of the SQLInsights tables, set this to true, but make sure you do this at least once after each update to the plugin

		// setup configuration for our dashboard
		DashboardConfig: &insights.DashboardConfig{
			TimeLocation: timeLocation, // set the time zone we want the dashboard to work under
		},
	}))
}
