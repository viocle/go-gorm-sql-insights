package insights

import (
	"sort"
	"time"
)

var (
	_statTypeQuery statType = "query"
	_statTypeRaw   statType = "raw"
)

type statType string

func (s statType) String() string {
	return string(s)
}

// stat defines a statistic to be collected, aggregated, and stored
type stat struct {
	TimeStamp  time.Time
	Type       statType
	Key        string
	KeyHash    string
	NumVars    int
	Took       float64
	Rows       int64
	Error      bool
	Callers    []*callerInfo
	CallerHash string
	CallerJSON []byte
}

// buildStatHistory builds a stat history from the specified list stat values
func (s *SQLInsights) buildStatHistory(now time.Time, keyHash string, sType statType, statValues []*stat) (*SQLInsightsHistory, []*SQLInsightsCallerHistory) {
	if len(statValues) <= 0 {
		return nil, nil
	}

	// build the stat and caller history (if enabled)
	callerHistory := make([]*SQLInsightsCallerHistory, 0, 5)
	callerHashMap := make(map[string]struct{}, 5)
	statHistory := &SQLInsightsHistory{
		InstanceID: s.instanceAppID,
		CreatedAt:  now,
		HashID:     keyHash,
		Type:       sType,
		Count:      len(statValues),
		TookMin:    -1,
		RowsMin:    -1,
	}
	// calculate the min, max, avg, and sum
	tookValues := make([]float64, 0, len(statValues))
	rowValues := make([]int64, 0, len(statValues))
	for _, statValue := range statValues {
		if statValue.Error {
			// increment the error count, skip the rest
			statHistory.Errors++
		} else {
			// collect raw values for median calculations
			tookValues = append(tookValues, statValue.Took)
			rowValues = append(rowValues, statValue.Rows)

			// Min/Max
			if statHistory.TookMin < 0 || statValue.Took < statHistory.TookMin {
				statHistory.TookMin = statValue.Took
			}
			if statValue.Took > statHistory.TookMax {
				statHistory.TookMax = statValue.Took
			}
			if statHistory.RowsMin < 0 || statValue.Rows < statHistory.RowsMin {
				statHistory.RowsMin = statValue.Rows
			}
			if statValue.Rows > statHistory.RowsMax {
				statHistory.RowsMax = statValue.Rows
			}

			// Sums
			statHistory.TookSum += statValue.Took
			statHistory.RowsSum += statValue.Rows
		}
		if statValue.CallerHash != "" && s.config.CollectCallerDepth > 0 {
			// we have a caller hash, so add it to the caller history if we haven't already
			if _, ok := callerHashMap[statValue.CallerHash]; !ok {
				callerHashMap[statValue.CallerHash] = struct{}{}
				callerHistoryValue := &SQLInsightsCallerHistory{
					ID:        statValue.CallerHash,
					CreatedAt: now,
					HashID:    keyHash,
				}
				callerHistoryValue.SetJSON(statValue.CallerJSON)
				callerHistory = append(callerHistory, callerHistoryValue)
			}
		}
	}

	validResults := len(statValues) - statHistory.Errors
	// calculate the avg and median
	if validResults > 0 {
		statHistory.TookAvg = statHistory.TookSum / float64(validResults)
		statHistory.RowsAvg = statHistory.RowsSum / int64(validResults)
		statHistory.TookMed = calculateMedianFloat64(tookValues)
		statHistory.RowsMed = calculateMedianInt64(rowValues)
	}

	return statHistory, callerHistory
}

// calculateMedianFloat64 calculates the median value from the specified list of float64 values. Does alter the sorce slice when sorting, which we dont care about here
func calculateMedianFloat64(values []float64) float64 {
	sort.Float64s(values)

	var median float64
	l := len(values)
	if l == 0 {
		return 0
	} else if l%2 == 0 {
		median = (values[l/2-1] + values[l/2]) / 2
	} else {
		median = values[l/2]
	}

	return median
}

// calculateMedianInt64 calculates the median value from the specified list of int64 values. Does alter the sorce slice when sorting, which we dont care about here
func calculateMedianInt64(values []int64) int64 {
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })

	var median int64
	l := len(values)
	if l == 0 {
		return 0
	} else if l%2 == 0 {
		median = (values[l/2-1] + values[l/2]) / 2
	} else {
		median = values[l/2]
	}

	return median
}
