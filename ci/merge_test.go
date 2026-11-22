//go:build ci
// +build ci

package ci

import (
	"testing"

	"github.com/alicebob/sqlittle"
)

// MergeRowid compares a merged stream over two shards against the same rows
// in a single database, as read by sqlite itself.
func TestMergeRowid(t *testing.T) {
	Compare(
		t,
		`
CREATE TABLE kv (k INTEGER PRIMARY KEY, v TEXT);
INSERT INTO kv VALUES (1, 'a1');
INSERT INTO kv VALUES (3, 'a3');
INSERT INTO kv VALUES (5, 'a5');
INSERT INTO kv VALUES (6, 'b6');
INSERT INTO kv VALUES (8, 'b8');
INSERT INTO kv VALUES (9, 'b9');
`,
		`SELECT k, v FROM kv ORDER BY k`,
		func(t *testing.T, db *sqlittle.DB) [][]string {
			var rows [][]string
			err := sqlittle.Merge(
				[]sqlittle.Shard{
					{Name: "a", Path: "../testdata/merge_a.sqlite"},
					{Name: "b", Path: "../testdata/merge_b.sqlite"},
				},
				sqlittle.MergeConfig{Table: "kv"},
				func(r sqlittle.MergedRow) bool {
					rows = append(rows, r.Row.ScanStrings())
					return false
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			return rows
		},
	)
}

// MergeWithoutRowid compares a merged WITHOUT ROWID stream against sqlite.
func TestMergeWithoutRowid(t *testing.T) {
	Compare(
		t,
		`
CREATE TABLE kv (k TEXT, n INTEGER, v TEXT, PRIMARY KEY (k, n)) WITHOUT ROWID;
INSERT INTO kv VALUES ('a', 1, 'a1');
INSERT INTO kv VALUES ('a', 3, 'a3');
INSERT INTO kv VALUES ('b', 2, 'b2');
INSERT INTO kv VALUES ('b', 5, 'b5');
INSERT INTO kv VALUES ('c', 1, 'c1');
`,
		`SELECT k, n, v FROM kv ORDER BY k, n`,
		func(t *testing.T, db *sqlittle.DB) [][]string {
			var rows [][]string
			err := sqlittle.Merge(
				[]sqlittle.Shard{
					{Name: "a", Path: "../testdata/merge_wr_a.sqlite"},
					{Name: "b", Path: "../testdata/merge_wr_b.sqlite"},
				},
				sqlittle.MergeConfig{Table: "kv"},
				func(r sqlittle.MergedRow) bool {
					rows = append(rows, r.Row.ScanStrings())
					return false
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			return rows
		},
	)
}

// MergeDescIndex merges two shards in descending index order.
func TestMergeDescIndex(t *testing.T) {
	Compare(
		t,
		`
CREATE TABLE kv (k TEXT, v TEXT);
INSERT INTO kv VALUES ('a', 'a');
INSERT INTO kv VALUES ('b', 'b');
INSERT INTO kv VALUES ('c', 'c');
INSERT INTO kv VALUES ('d', 'd');
INSERT INTO kv VALUES ('e', 'e');
INSERT INTO kv VALUES ('f', 'f');
`,
		`SELECT k, v FROM kv ORDER BY k DESC`,
		func(t *testing.T, db *sqlittle.DB) [][]string {
			var rows [][]string
			err := sqlittle.Merge(
				[]sqlittle.Shard{
					{Name: "a", Path: "../testdata/merge_idx_a.sqlite"},
					{Name: "b", Path: "../testdata/merge_idx_b.sqlite"},
				},
				sqlittle.MergeConfig{Table: "kv", Index: "kv_k"},
				func(r sqlittle.MergedRow) bool {
					rows = append(rows, r.Row.ScanStrings())
					return false
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			return rows
		},
	)
}
