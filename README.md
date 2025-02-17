# GORM SQL Insights Plugin
[![Go Report Card](https://goreportcard.com/badge/github.com/viocle/go-gorm-sql-insights)](https://goreportcard.com/report/github.com/viocle/go-gorm-sql-insights) 
[![GoDoc](https://godoc.org/github.com/viocle/go-gorm-sql-insights?status.svg)](https://godoc.org/github.com/viocle/go-gorm-sql-insights)
[![tests](https://github.com/viocle/go-gorm-sql-insights/actions/workflows/tests.yml/badge.svg)](https://github.com/viocle/go-gorm-sql-insights/actions/workflows/tests.yml)

This is a [GORM](https://gorm.io/index.html) plugin to help gather insights into SQL queries being performed. The goal of this project is to make it easier to see the frequency of queries, their timing, and the resources consumed around them giving you the information to be better informed on queries that may need to be optimized through restructuring the SQL statement, changes to indexing, or utilize caching/dataloader technologies.

## How to use
Register this plugin using the `Use` method on the `*gorm.DB` instance you want to monitor. Example, add a generic addPlugins function to load your plugins, passing your *gorm.DB reference:
```
import (
	"time"

	insights "github.com/viocle/go-gorm-sql-insights/plugin"
	"gorm.io/gorm"
)

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

		// setup configuration for our dashboard
		DashboardConfig: &insights.DashboardConfig{
			TimeLocation: timeLocation, // set the time zone we want the dashboard to work under
		},
	}))
}
```

## Benchmarks
Run benchmarks with profiling from the plugin directory
```
go test -benchmem -run=^$ -bench ^BenchmarkSQLInsights$ -cpuprofile=cpu -memprofile=mem
```
Benchmark Results:
```
plugin\> go version
go version go1.24.0 windows/amd64
plugin\> go test -benchmem -run=^$ -bench ^BenchmarkSQLInsights$
goos: windows
goarch: amd64
pkg: github.com/viocle/go-gorm-sql-insights/plugin
cpu: AMD Ryzen 9 5900X 12-Core Processor
BenchmarkSQLInsights/queryNoHooks-24              331490              3390 ns/op            3420 B/op         44 allocs/op
BenchmarkSQLInsights/queryHooks-24                174385              6006 ns/op            4439 B/op         65 allocs/op
PASS
ok      github.com/viocle/go-gorm-sql-insights/plugin   5.696s
```
```
> go version
go version go1.24.0 windows/amd64
parser\> go test -benchmem -run=^$ -bench ^BenchmarkParseSQL$
goos: windows
goarch: amd64
pkg: github.com/viocle/go-gorm-sql-insights/parser
cpu: AMD Ryzen 9 5900X 12-Core Processor
BenchmarkParseSQL-24                6136            172251 ns/op          112533 B/op        995 allocs/op
PASS
ok      github.com/viocle/go-gorm-sql-insights/parser   1.440s
```