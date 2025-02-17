package parser

import (
	"encoding/json"
	"os"
	"testing"
)

type testSQLParsingTests struct {
	SQL    string
	Result ParsedFields
}

// BenchmarkParseSQL will benchmark the ParseSQL function using examples in sqlParsingTests.json
func BenchmarkParseSQL(b *testing.B) {
	// load test example data from sqlParsingTest.json
	testData, err := os.ReadFile("./sqlParsingTests.json")
	if err != nil {
		b.Fatalf("Error reading test example data: %v", err)
	}
	var tests []testSQLParsingTests
	if err := json.Unmarshal(testData, &tests); err != nil {
		b.Fatalf("Error unmarshalling test example data: %v", err)
	}
	if len(tests) == 0 {
		b.Fatalf("No test data found")
	}
	b.ResetTimer()
	// create our new parser
	parser, err := New(Config{})
	if err != nil {
		b.Fatalf("Error creating parser: %v", err)
	}
	// run our tests in this benchmark
	for i := 0; i < b.N; i++ {
		for _, test := range tests {
			parser.ParseSQL(test.SQL)
		}
	}
}

func TestSQLParsing(t *testing.T) {
	// load test example data from sqlParsingTest.json
	testData, err := os.ReadFile("./sqlParsingTests.json")
	if err != nil {
		t.Fatalf("Error reading test example data: %v", err)
	}
	// unmarshal the test data
	var tests []testSQLParsingTests
	if err := json.Unmarshal(testData, &tests); err != nil {
		t.Fatalf("Error unmarshalling test example data: %v", err)
	}
	if len(tests) == 0 {
		t.Fatalf("No test data found")
	}
	// create our new parser
	parser, err := New(Config{})
	if err != nil {
		t.Fatalf("Error creating parser: %v", err)
	}
	// run our tests
	for idx, test := range tests {
		f, err := parser.ParseSQL(test.SQL)
		if err != nil {
			t.Fatalf("Error parsing SQL: %v", err)
		} else if f == nil {
			t.Fatalf("Error parsing SQL: no fields returned")
		}
		if !f.Equal(&test.Result) {
			t.Errorf("Error parsing SQL: field results do not match for test [%d]", idx)
			t.Errorf("SQL: %s", test.SQL)
			t.Errorf("Expected: %+v", test.Result)
			t.Errorf("Got: %+v", *f)
		}
	}
}
