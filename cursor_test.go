package sqlittle

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	sdb "github.com/alicebob/sqlittle/db"
	"github.com/stretchr/testify/require"
)

// cursorTestDB opens a database and holds a read lock for the test.
func cursorTestDB(t *testing.T, path string) *DB {
	t.Helper()
	db, err := Open(path)
	require.NoError(t, err)
	require.NoError(t, db.db.RLock())
	t.Cleanup(func() {
		db.db.RUnlock()
		db.Close()
	})
	return db
}

func testFiles(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob("testdata/*.sqlite")
	require.NoError(t, err)
	var res []string
	for _, f := range files {
		// skip the intentionally broken files; Open() is tested elsewhere
		base := filepath.Base(f)
		if strings.HasPrefix(base, "notadatabase") ||
			strings.HasPrefix(base, "truncated") ||
			strings.HasPrefix(base, "zerolength") ||
			strings.HasPrefix(base, "wal") {
			continue
		}
		res = append(res, f)
	}
	sort.Strings(res)
	require.NotEmpty(t, res)
	return res
}

// TestTableCursorMatchesScan reads every table in every test database with
// both the callback API and the cursor API, and checks they agree.
func TestTableCursorMatchesScan(t *testing.T) {
	for _, path := range testFiles(t) {
		db, err := Open(path)
		if err != nil {
			continue // unusable for a scan-only comparison
		}
		_ = db.db.RLock()
		tables, err := db.db.Tables()
		if err != nil {
			db.db.RUnlock()
			db.Close()
			continue // e.g. issue_1.sqlite has an unparsable master table
		}

		for _, table := range tables {
			s, err := db.db.Schema(table)
			if err != nil || s.WithoutRowid {
				continue
			}
			tab, err := db.db.Table(table)
			if err != nil {
				continue
			}

			var want []sdb.Record
			var wantRowids []int64
			if err := tab.Scan(func(rowid int64, rec sdb.Record) bool {
				wantRowids = append(wantRowids, rowid)
				want = append(want, rec)
				return false
			}); err != nil {
				continue // corrupt fixture; Scan's own error is tested elsewhere
			}

			c, err := tab.Cursor()
			require.NoError(t, err)
			var have []sdb.Record
			var haveRowids []int64
			for {
				rowid, rec, ok, err := c.Next()
				require.NoError(t, err, "%s %s", path, table)
				if !ok {
					break
				}
				haveRowids = append(haveRowids, rowid)
				have = append(have, rec)
			}
			require.Equal(t, wantRowids, haveRowids, "%s %s", path, table)
			require.Equal(t, want, have, "%s %s", path, table)
		}
		db.db.RUnlock()
		db.Close()
	}
}

// TestIndexCursorMatchesScan does the same for every index in every test
// database, and for every WITHOUT ROWID table (which is stored as an index).
func TestIndexCursorMatchesScan(t *testing.T) {
	for _, path := range testFiles(t) {
		db, err := Open(path)
		if err != nil {
			continue
		}
		_ = db.db.RLock()

		names, err := db.db.Indexes()
		if err != nil {
			db.db.RUnlock()
			db.Close()
			continue
		}
		tables, err := db.db.Tables()
		if err != nil {
			db.db.RUnlock()
			db.Close()
			continue
		}
		for _, table := range tables {
			if s, err := db.db.Schema(table); err == nil && s.WithoutRowid {
				names = append(names, table)
			}
		}

		for _, name := range names {
			ind, err := db.db.Index(name)
			if err != nil {
				// WITHOUT ROWID tables need NonRowidTable()
				ind, err = db.db.NonRowidTable(name)
			}
			require.NoError(t, err, "%s %s", path, name)

			var want []sdb.Record
			if err := ind.Scan(func(rec sdb.Record) bool {
				want = append(want, rec)
				return false
			}); err != nil {
				continue // corrupt fixture
			}

			cur, err := ind.Cursor()
			require.NoError(t, err)
			var have []sdb.Record
			for {
				rec, ok, err := cur.Next()
				require.NoError(t, err, "%s %s", path, name)
				if !ok {
					break
				}
				have = append(have, rec)
			}
			require.Equal(t, want, have, "%s %s", path, name)
		}
		db.db.RUnlock()
		db.Close()
	}
}

func TestCursorNextAfterDone(t *testing.T) {
	db := cursorTestDB(t, "testdata/single.sqlite")
	tab, err := db.db.Table("hello")
	require.NoError(t, err)
	c, err := tab.Cursor()
	require.NoError(t, err)
	var n int
	for {
		_, _, ok, err := c.Next()
		require.NoError(t, err)
		if !ok {
			break
		}
		n++
	}
	require.NotZero(t, n)
	for i := 0; i < 3; i++ {
		_, _, ok, err := c.Next()
		require.NoError(t, err)
		require.False(t, ok)
	}
}

func TestCursorBigTable(t *testing.T) {
	// words.sqlite has 1000 rows: multiple leaf pages and an interior level
	db := cursorTestDB(t, "testdata/words.sqlite")
	tab, err := db.db.Table("words")
	require.NoError(t, err)
	c, err := tab.Cursor()
	require.NoError(t, err)

	var n int
	var last int64
	for {
		rowid, _, ok, err := c.Next()
		require.NoError(t, err)
		if !ok {
			break
		}
		require.True(t, rowid > last, fmt.Sprintf("rowid %d after %d", rowid, last))
		last = rowid
		n++
	}
	require.Equal(t, 1000, n)
}
