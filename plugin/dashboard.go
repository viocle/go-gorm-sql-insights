package insights

import (
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// DashboardConfig defines the configuration for the SQLInsights dashboard user interface
type DashboardConfig struct {
	// TimeLocation is the time location to use for all time related operations
	TimeLocation *time.Location
}

// DashboardMux returns a new ServeMux with the dashboard and API handlers registered
func (s *SQLInsights) DashboardMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/api", s.apiHandler())
	mux.HandleFunc("/", s.dashboardHandler())
	return mux
}

// DashboardHandler handles HTTP requests for the dashboard interface to access SQL insights data through this SQLInsights instance
func (s *SQLInsights) dashboardHandler() func(w http.ResponseWriter, r *http.Request) {
	// return file server handling for the contents of the web directory
	return http.FileServer(http.Dir("./web")).ServeHTTP
}

// APIHandler handles HTTP requests for the API interface to access SQL insights data through this SQLInsights instance
func (s *SQLInsights) apiHandler() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		// handle the request
		request := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("request")))
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		switch request {
		case "sql_query_counts":
			// handle the SQLQueryCounts request
			var input SQLQueryCountsRequest
			if err := json.Unmarshal(body, &input); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			// get the counts
			results, err := s.SQLQueryCounts(&input)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			// write the response
			if err := json.NewEncoder(w).Encode(results); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		case "sql_query_history":
			// handle the SQLQueryHistory request
			var input SQLQueryHistoryRequest
			if err := json.Unmarshal(body, &input); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			// get the history
			results, err := s.SQLQueryHistory(&input)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			// write the response
			if err := json.NewEncoder(w).Encode(results); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
	}
}

// SQLQueryCountsResult defines the result of the SQLQueryCounts method
type SQLQueryCountsResult struct {
	InstanceAppID string
	InstanceID    int
	Counts        []*SQLQueryCountsResultDaySummary
}

// SQLQueryCountsResultDaySummary defines the result of the SQLQueryCounts method for a single day
type SQLQueryCountsResultDaySummary struct {
	Day   time.Time
	Count int
}

// SQLQueryCountsRequest defines the input for the SQLQueryCounts method
type SQLQueryCountsRequest struct {
	InstanceAppIDs []string
	From           *time.Time
	To             *time.Time
}

type SQLQueryHistoryRequest struct {
	InstanceAppIDs []string
	From           *time.Time
	To             *time.Time
}

// sqlInsightsQueryCountsDBResult defines the result of the SQL query inside SQLQueryCounts method
type SQLInsightsQueryQueryHistoryDBResult struct {
	SQLInsightsHistory
	InstanceAppName string
}

// SQLQueryHistory returns the history of SQL queries executed over a period of time to be graphed
func (s *SQLInsights) SQLQueryHistory(input *SQLQueryHistoryRequest) ([]*SQLInsightsQueryQueryHistoryDBResult, error) {
	if input == nil {
		return nil, nil
	}

	// setup our time range
	var fromTime, toTime time.Time
	if input.From != nil && !input.From.IsZero() {
		fromTime = *input.From
		fromTime = fromTime.UTC()
	} else {
		// default to 7 days ago
		fromTime = time.Now().UTC().AddDate(0, 0, -7)
	}
	if input.To != nil && !input.To.IsZero() {
		toTime = *input.To
		toTime = toTime.UTC()
	} else {
		// default to now
		toTime = time.Now().UTC()
	}

	// create query
	query := s.StatDB().Select("sql_insights_app.instance_app_name, sql_insights_history.*")

	// join on the SQLInsightsApp table to get the InstanceID
	query = query.Joins("INNER JOIN sql_insights_app ON sql_insights_history.instance_id = sql_insights_app.id")
	query = query.Model(&SQLInsightsHistory{}).Where("created_at >= ? AND created_at <= ?", fromTime, toTime)
	if len(input.InstanceAppIDs) > 0 {
		query = query.Where("instance_id IN (?)", input.InstanceAppIDs)
	}

	// get the counts
	var results []*SQLInsightsQueryQueryHistoryDBResult
	if err := query.Find(&results).Error; err != nil {
		return nil, err
	}

	return results, nil
}

// SQLQueryCounts returns the number of SQL queries executed per day per ApplicationID over a period of time to be graphed
func (s *SQLInsights) SQLQueryCounts(input *SQLQueryCountsRequest) ([]*SQLQueryCountsResult, error) {
	if input == nil {
		return nil, nil
	}

	// query history
	results, err := s.SQLQueryHistory(&SQLQueryHistoryRequest{
		InstanceAppIDs: input.InstanceAppIDs,
		From:           input.From,
		To:             input.To,
	})
	if err != nil {
		return nil, err
	}

	// group the results by InstanceAppID
	groupedResults := make(map[string][]*SQLInsightsHistory)
	for _, result := range results {
		if results == nil {
			continue
		}
		if _, ok := groupedResults[result.InstanceAppName]; !ok {
			groupedResults[result.InstanceAppName] = make([]*SQLInsightsHistory, 0, 10)
		}
		groupedResults[result.InstanceAppName] = append(groupedResults[result.InstanceAppName], &result.SQLInsightsHistory)
	}

	// build the result
	var finalResults []*SQLQueryCountsResult
	for instanceAppName, instanceHistories := range groupedResults {
		if len(instanceHistories) <= 0 {
			continue
		}
		// group the histories by day
		groupedHistories := make(map[time.Time]int)
		for _, history := range instanceHistories {
			// get the day
			createdAt := history.CreatedAt.In(s.config.DashboardConfig.TimeLocation)
			day := time.Date(createdAt.Year(), createdAt.Month(), createdAt.Day(), 0, 0, 0, 0, s.config.DashboardConfig.TimeLocation)
			if _, ok := groupedHistories[day]; !ok {
				groupedHistories[day] = 1
			} else {
				groupedHistories[day]++
			}
		}
		// build the result
		result := &SQLQueryCountsResult{
			InstanceAppID: instanceAppName,
			InstanceID:    instanceHistories[0].InstanceID,
			Counts:        make([]*SQLQueryCountsResultDaySummary, 0, len(groupedHistories)),
		}
		for day, count := range groupedHistories {
			result.Counts = append(result.Counts, &SQLQueryCountsResultDaySummary{
				Day:   day,
				Count: count,
			})
		}
		// sort the result by day
		sort.Slice(result.Counts, func(i, j int) bool {
			return result.Counts[i].Day.Before(result.Counts[j].Day)
		})
		finalResults = append(finalResults, result)
	}

	return finalResults, nil
}
