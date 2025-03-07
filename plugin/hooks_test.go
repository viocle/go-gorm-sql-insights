package insights

import (
	"database/sql"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func BenchmarkSQLInsights(b *testing.B) {
	sqlDB, db, _ := newMock(nil, b)
	defer sqlDB.Close()

	// create our new insights monitor
	sInsights := New(Config{
		DB:                     db,
		InstanceID:             "test",
		CollectCallerDepth:     5,
		CollectSystemResources: true,
	})

	// test query without our plugin hooks
	b.Run("queryNoHooks", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			db.Where("id = ?", i).Order("id ASC").Find(&mockTestUser{})
		}
	})

	// add our plugin
	db.Use(sInsights)

	// test query with our plugin hooks
	b.Run("queryHooks", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = db.Where("id = ?", i).Order("id ASC").Find(&mockTestUser{}).Error
		}
	})

	// stop insights
	if err := sInsights.Stop(0); err != nil {
		b.Fatalf("failed to stop sql insights plugin: %s", err)
	}
}

func TestSQLInsights(t *testing.T) {
	sqlDB, db, _ := newMock(t, nil)
	defer sqlDB.Close()

	// create our new insights monitor
	sInsights := New(Config{
		DB:                     db,
		InstanceID:             "test",
		CollectCallerDepth:     5,
		CollectSystemResources: true,
	})

	// compare Config matches the expected values
	if sInsights.config.InstanceID != "test" {
		t.Fatalf("expected instance ID to be 'test', got %s", sInsights.config.InstanceID)
	}
	if sInsights.config.CollectCallerDepth != 5 {
		t.Fatalf("expected CollectCallerDepth to be 5, got %d", sInsights.config.CollectCallerDepth)
	}
	if sInsights.config.CollectSystemResources != true {
		t.Fatalf("expected CollectSystemResources to be true, got %t", sInsights.config.CollectSystemResources)
	}
	if sInsights.config.AutoPurgeAge != 0 {
		t.Fatalf("expected AutoPurgeAge to be 0, got %d", sInsights.config.AutoPurgeAge)
	}
	if sInsights.config.StopTimeLimit != 0 {
		t.Fatalf("expected StopTimeLimit to be 0, got %d", sInsights.config.StopTimeLimit)
	}
	if sInsights.config.SkipAutomigration != false {
		t.Fatalf("expected SkipAutomigration to be false, got %t", sInsights.config.SkipAutomigration)
	}

	// setup plugin
	db.Use(sInsights)

	wg := sync.WaitGroup{}
	wg.Add(10)
	for idx := 0; idx < 10; idx++ {
		go func(i int) {
			defer wg.Done()
			db.Where("id = ?", i).Order("id ASC").Find(&mockTestUser{})
		}(idx)
	}
	wg.Wait()

	// give time for background workers to process
	time.Sleep(10 * time.Millisecond)

	// drain the stats channel
	sInsights.DrainStatsChannel(10 * time.Second)

	// we should have 1 hash and 10 stats entries
	sInsights.statsLock.Lock()
	if len(sInsights.stats) != 1 {
		t.Fatalf("expected 1 statement type entry, got %d", len(sInsights.stats))
	}
	if statements := len(sInsights.stats[_statTypeQuery]); statements != 1 {
		t.Fatalf("expected 1 statement hash entry, got %d", statements)
	}
	for _, stats := range sInsights.stats[_statTypeQuery] {
		if len(stats) != 10 {
			t.Fatalf("expected 10 stats entries, got %d", len(stats))
		}
	}
	sInsights.statsLock.Unlock()

	// stop insights
	if err := sInsights.Stop(0); err != nil {
		t.Fatalf("failed to stop sql insights plugin: %s", err)
	}
}

type mockTestUser struct {
	ID       uint   `gorm:"primarykey"`
	FullName string `json:"full_name"`
	UserName string `json:"user_name"`
	Password string `json:"password"`
}

// newMock creates a new mock database and gorm connection for testing/benchmarking purposes. Ignore expectations
func newMock(t *testing.T, b *testing.B) (*sql.DB, *gorm.DB, sqlmock.Sqlmock) {
	sqldb, mock, err := sqlmock.New()
	if err != nil {
		if t != nil {
			t.Fatal(err)
		} else if b != nil {
			b.Fatal(err)
		}
	}
	gormdb, err := gorm.Open(postgres.New(postgres.Config{
		Conn: sqldb,
	}), &gorm.Config{
		// silence log output, we're not using expectations
		Logger: logger.Default.LogMode(logger.Silent),
	})

	if err != nil {
		if t != nil {
			t.Fatal(err)
		} else if b != nil {
			b.Fatal(err)
		}
	}

	return sqldb, gormdb, mock
}
