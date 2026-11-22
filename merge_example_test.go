package sqlittle_test

import (
	"fmt"

	"github.com/alicebob/sqlittle"
)

// Merge two shards into one ordered stream, keeping the shard origin.
func ExampleMerge() {
	err := sqlittle.Merge(
		[]sqlittle.Shard{
			{Name: "a", Path: "./testdata/merge_a.sqlite"},
			{Name: "b", Path: "./testdata/merge_b.sqlite"},
		},
		sqlittle.MergeConfig{Table: "kv"},
		func(r sqlittle.MergedRow) bool {
			fmt.Printf("%s: %v\n", r.Shard, r.Row.ScanStrings())
			return false
		},
	)
	if err != nil {
		panic(err)
	}
	// output:
	// a: [1 a1]
	// a: [3 a3]
	// a: [5 a5]
	// b: [6 b6]
	// b: [8 b8]
	// b: [9 b9]
}

// Verify the key order and column layout of a set of shards.
func ExampleVerifyShards() {
	layout, err := sqlittle.VerifyShards(
		[]sqlittle.Shard{
			{Name: "a", Path: "./testdata/merge_a.sqlite"},
			{Name: "b", Path: "./testdata/merge_b.sqlite"},
		},
		sqlittle.MergeConfig{Table: "kv"},
	)
	if err != nil {
		panic(err)
	}
	fmt.Printf("%s %v %v\n", layout.Table, layout.Columns, layout.KeyColumns)
	// output:
	// kv [k v] [k]
}
