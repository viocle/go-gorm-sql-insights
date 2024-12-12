# GORM SQL Insights Plugin

## Benchmarks
Run benchmarks with profiling from the plugin directory
```
go test -benchmem -run=^$ -bench ^BenchmarkSQLInsights$ -cpuprofile=cpu -memprofile=mem
```
Benchmark Results:
```
> go test -benchmem -run=^$ -bench ^BenchmarkSQLInsights$ -cpuprofile=cpu -memprofile=mem
goos: windows
goarch: amd64
pkg: github.com/viocle/go-gorm-sql-insights/plugin
cpu: AMD Ryzen 9 5900X 12-Core Processor
BenchmarkSQLInsights/queryNoHooks-24              306026              3874 ns/op            3414 B/op         47 allocs/op
BenchmarkSQLInsights/queryHooks-24                164044              7452 ns/op            4481 B/op         72 allocs/op
PASS
ok      github.com/viocle/go-gorm-sql-insights/plugin   5.732s
```