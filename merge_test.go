package sqlittle

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func mergedRows(t *testing.T, shards []Shard, cfg MergeConfig) ([]MergedRow, Layout, error) {
	t.Helper()
	m, err := NewMerger(shards, cfg)
	if err != nil {
		return nil, Layout{}, err
	}
	defer m.Close()

	var rows []MergedRow
	for {
		r, ok, err := m.Next()
		if err != nil {
			return rows, m.Layout(), err
		}
		if !ok {
			break
		}
		rows = append(rows, r)
	}
	return rows, m.Layout(), nil
}

func rowStrings(rows []MergedRow) [][]string {
	res := make([][]string, len(rows))
	for i, r := range rows {
		res[i] = append([]string{r.Shard}, r.Row.ScanStrings()...)
	}
	return res
}

func TestMergeRowid(t *testing.T) {
	shards := []Shard{
		{Name: "a", Path: "testdata/merge_a.sqlite"},
		{Name: "b", Path: "testdata/merge_b.sqlite"},
	}
	rows, layout, err := mergedRows(t, shards, MergeConfig{Table: "kv"})
	require.NoError(t, err)

	require.Equal(t, "kv", layout.Table)
	require.Equal(t, []string{"k", "v"}, layout.Columns)
	require.Equal(t, []string{"INTEGER", "TEXT"}, layout.Types)
	require.Equal(t, []string{"k"}, layout.KeyColumns)
	require.Equal(t, []string{""}, layout.KeyCollations)
	require.Equal(t, []bool{false}, layout.KeyDesc)
	require.False(t, layout.WithoutRowid)

	require.Equal(t, [][]string{
		{"a", "1", "a1"},
		{"a", "3", "a3"},
		{"a", "5", "a5"},
		{"b", "6", "b6"},
		{"b", "8", "b8"},
		{"b", "9", "b9"},
	}, rowStrings(rows))

	// keys and origins
	require.Equal(t, Key{int64(1)}, rows[0].Key)
	require.Equal(t, 0, rows[0].ShardIndex)
	require.Equal(t, Key{int64(9)}, rows[5].Key)
	require.Equal(t, 1, rows[5].ShardIndex)
}

func TestMergeShardOrderDoesNotMatter(t *testing.T) {
	cfg := MergeConfig{Table: "kv", Columns: []string{"v"}}
	ab, _, err := mergedRows(t, []Shard{
		{Name: "a", Path: "testdata/merge_a.sqlite"},
		{Name: "b", Path: "testdata/merge_b.sqlite"},
	}, cfg)
	require.NoError(t, err)

	ba, _, err := mergedRows(t, []Shard{
		{Name: "b", Path: "testdata/merge_b.sqlite"},
		{Name: "a", Path: "testdata/merge_a.sqlite"},
	}, cfg)
	require.NoError(t, err)

	require.Equal(t, rowStrings(ab), rowStrings(ba))
}

func TestMergeColumnSelection(t *testing.T) {
	rows, _, err := mergedRows(t, []Shard{
		{Name: "a", Path: "testdata/merge_a.sqlite"},
	}, MergeConfig{Table: "kv", Columns: []string{"v"}})
	require.NoError(t, err)
	require.Equal(t, [][]string{
		{"a", "a1"},
		{"a", "a3"},
		{"a", "a5"},
	}, rowStrings(rows))

	_, _, err = mergedRows(t, []Shard{
		{Name: "a", Path: "testdata/merge_a.sqlite"},
	}, MergeConfig{Table: "kv", Columns: []string{"nosuch"}})
	require.EqualError(t, err, `shard "a": no such column: "nosuch"`)
}

func TestMergeWithoutRowid(t *testing.T) {
	rows, layout, err := mergedRows(t, []Shard{
		{Name: "a", Path: "testdata/merge_wr_a.sqlite"},
		{Name: "b", Path: "testdata/merge_wr_b.sqlite"},
	}, MergeConfig{Table: "kv"})
	require.NoError(t, err)
	require.True(t, layout.WithoutRowid)
	require.Equal(t, []string{"k", "n"}, layout.KeyColumns)

	require.Equal(t, [][]string{
		{"a", "a", "1", "a1"},
		{"a", "a", "3", "a3"},
		{"a", "b", "2", "b2"},
		{"b", "b", "5", "b5"},
		{"b", "c", "1", "c1"},
	}, rowStrings(rows))
	require.Equal(t, Key{"a", int64(1)}, rows[0].Key)
}

func TestMergeWithoutRowidIndex(t *testing.T) {
	rows, layout, err := mergedRows(t, []Shard{
		{Name: "a", Path: "testdata/merge_wr_idx_a.sqlite"},
		{Name: "b", Path: "testdata/merge_wr_idx_b.sqlite"},
	}, MergeConfig{Table: "kv", Index: "kv_score"})
	require.NoError(t, err)
	require.Equal(t, []string{"score"}, layout.KeyColumns)

	require.Equal(t, [][]string{
		{"a", "a", "10", "a"},
		{"b", "b", "20", "b"},
		{"a", "c", "30", "c"},
		{"b", "d", "40", "d"},
		{"a", "e", "50", "e"},
		{"b", "f", "60", "f"},
	}, rowStrings(rows))
}

func TestMergeBigInterleaved(t *testing.T) {
	// 1000 rows over two shards, keys interleaved, so the merge has to
	// alternate almost every row across multi-page btrees.
	rows, _, err := mergedRows(t, []Shard{
		{Name: "a", Path: "testdata/merge_big_a.sqlite"},
		{Name: "b", Path: "testdata/merge_big_b.sqlite"},
	}, MergeConfig{Table: "kv", Columns: []string{"v"}})
	require.NoError(t, err)
	require.Len(t, rows, 1000)

	for i, r := range rows {
		require.Equal(t, int64(i), r.Key[0], "row %d", i)
		wantShard := "a"
		if i%2 == 1 {
			wantShard = "b"
		}
		require.Equal(t, wantShard, r.Shard, "row %d", i)
	}
}

func TestMergeCollate(t *testing.T) {
	// The shards are split so that the shard boundary falls between the keys
	// "grape" and "Fig", which only sorts correctly with the NOCASE collation
	// of the primary key column.
	rows, layout, err := mergedRows(t, []Shard{
		{Name: "a", Path: "testdata/merge_collate_a.sqlite"},
		{Name: "b", Path: "testdata/merge_collate_b.sqlite"},
	}, MergeConfig{Table: "kv"})
	require.NoError(t, err)
	require.Equal(t, []string{"nocase"}, layout.KeyCollations)
	require.Equal(t, [][]string{
		{"a", "apple", "a-apple"},
		{"b", "banana", "b-banana"},
		{"a", "Cherry", "a-cherry"},
		{"b", "date", "b-date"},
		{"b", "Fig", "b-fig"},
		{"a", "grape", "a-grape"},
	}, rowStrings(rows))
}

func TestMergeDescIndex(t *testing.T) {
	rows, layout, err := mergedRows(t, []Shard{
		{Name: "a", Path: "testdata/merge_idx_a.sqlite"},
		{Name: "b", Path: "testdata/merge_idx_b.sqlite"},
	}, MergeConfig{Table: "kv", Index: "kv_k"})
	require.NoError(t, err)
	require.Equal(t, []string{"k"}, layout.KeyColumns)
	require.Equal(t, []bool{true}, layout.KeyDesc)

	require.Equal(t, [][]string{
		{"b", "f", "f"},
		{"a", "e", "e"},
		{"b", "d", "d"},
		{"a", "c", "c"},
		{"b", "b", "b"},
		{"a", "a", "a"},
	}, rowStrings(rows))
}

func TestMergeEmptyShard(t *testing.T) {
	rows, _, err := mergedRows(t, []Shard{
		{Name: "empty", Path: "testdata/merge_empty.sqlite"},
		{Name: "a", Path: "testdata/merge_a.sqlite"},
	}, MergeConfig{Table: "kv"})
	require.NoError(t, err)
	require.Len(t, rows, 3)
	require.Equal(t, "a", rows[0].Shard)
}

func TestMergeNoSuchTable(t *testing.T) {
	_, _, err := mergedRows(t, []Shard{
		{Name: "a", Path: "testdata/merge_a.sqlite"},
	}, MergeConfig{Table: "nosuch"})
	require.EqualError(t, err, `shard "a": no such table: "nosuch"`)
}

func TestMergeNoSuchIndex(t *testing.T) {
	_, _, err := mergedRows(t, []Shard{
		{Name: "a", Path: "testdata/merge_a.sqlite"},
	}, MergeConfig{Table: "kv", Index: "nosuch"})
	require.EqualError(t, err, `shard "a": no such index: "nosuch"`)
}

func TestMergeColumnLayoutMismatch(t *testing.T) {
	_, _, err := mergedRows(t, []Shard{
		{Name: "a", Path: "testdata/merge_a.sqlite"},
		{Name: "bad", Path: "testdata/merge_layout_bad.sqlite"},
	}, MergeConfig{Table: "kv"})
	require.ErrorIs(t, err, ErrShardLayout)
	require.Contains(t, err.Error(), `shard "bad"`)
}

func TestMergeKeyLayoutMismatch(t *testing.T) {
	// same columns, but no primary key, so the merge key is the rowid
	_, _, err := mergedRows(t, []Shard{
		{Name: "a", Path: "testdata/merge_conflict_a.sqlite"},
		{Name: "nopk", Path: "testdata/merge_nopk.sqlite"},
	}, MergeConfig{Table: "kv"})
	require.ErrorIs(t, err, ErrShardLayout)
	require.Contains(t, err.Error(), `shard "nopk"`)
}

func TestMergeConflictError(t *testing.T) {
	shards := []Shard{
		{Name: "a", Path: "testdata/merge_conflict_a.sqlite"},
		{Name: "b", Path: "testdata/merge_conflict_b.sqlite"},
	}
	m, err := NewMerger(shards, MergeConfig{Table: "kv"})
	require.NoError(t, err)
	defer m.Close()

	var keys []string
	for {
		r, ok, err := m.Next()
		if err != nil {
			require.ErrorIs(t, err, ErrBoundaryConflict)
			require.Contains(t, err.Error(), "m")
			require.Contains(t, err.Error(), `"a"`)
			require.Contains(t, err.Error(), `"b"`)
			// the error is sticky
			_, _, err2 := m.Next()
			require.Equal(t, err, err2)
			break
		}
		if !ok {
			t.Fatal("expected conflict error")
		}
		keys = append(keys, r.Key[0].(string))
	}
	// rows before the conflicting key are returned
	require.Equal(t, []string{"a", "b"}, keys)
}

func TestMergeConflictPriority(t *testing.T) {
	rows, _, err := mergedRows(t, []Shard{
		{Name: "low", Path: "testdata/merge_conflict_a.sqlite", Priority: 1},
		{Name: "high", Path: "testdata/merge_conflict_b.sqlite", Priority: 5},
	}, MergeConfig{Table: "kv", Conflict: ConflictPriority})
	require.NoError(t, err)
	require.Equal(t, [][]string{
		{"low", "a", "a"},
		{"high", "b", "b"},
		{"high", "m", "from-b"},
		{"high", "y", "y"},
		{"low", "z", "z"},
	}, rowStrings(rows))
}

func TestMergeConflictPriorityEqual(t *testing.T) {
	// equal priorities: the earliest registered shard wins
	rows, _, err := mergedRows(t, []Shard{
		{Name: "first", Path: "testdata/merge_conflict_b.sqlite"},
		{Name: "second", Path: "testdata/merge_conflict_a.sqlite"},
	}, MergeConfig{Table: "kv", Conflict: ConflictPriority})
	require.NoError(t, err)

	var m MergedRow
	for _, r := range rows {
		if r.Key[0] == "m" {
			m = r
		}
	}
	require.Equal(t, "first", m.Shard)
	require.Equal(t, "from-b", m.Row.ScanStrings()[1])
}

func TestMergeKeyOrderViolation(t *testing.T) {
	// non-unique index: duplicate keys within one shard
	_, _, err := mergedRows(t, []Shard{
		{Name: "dup", Path: "testdata/merge_dup.sqlite"},
	}, MergeConfig{Table: "kv", Index: "kv_k"})
	require.ErrorIs(t, err, ErrKeyOrder)
	require.Contains(t, err.Error(), `shard "dup"`)
}

func TestMergeInvalidConfig(t *testing.T) {
	good := Shard{Name: "a", Path: "testdata/merge_a.sqlite"}

	_, err := NewMerger(nil, MergeConfig{Table: "kv"})
	require.EqualError(t, err, "no shards given")

	_, err = NewMerger([]Shard{good}, MergeConfig{})
	require.EqualError(t, err, "MergeConfig.Table is required")

	_, err = NewMerger([]Shard{{Name: "", Path: "x"}}, MergeConfig{Table: "kv"})
	require.Error(t, err)

	_, err = NewMerger([]Shard{{Name: "a", Path: ""}}, MergeConfig{Table: "kv"})
	require.Error(t, err)

	_, err = NewMerger([]Shard{good, good}, MergeConfig{Table: "kv"})
	require.EqualError(t, err, `duplicate shard name: "a"`)

	samePath := Shard{Name: "b", Path: good.Path}
	_, err = NewMerger([]Shard{good, samePath}, MergeConfig{Table: "kv"})
	require.EqualError(t, err, `duplicate shard path: "testdata/merge_a.sqlite"`)

	_, err = NewMerger([]Shard{good}, MergeConfig{Table: "kv", Conflict: ConflictPolicy(42)})
	require.Error(t, err)
}

func TestMergeMissingFile(t *testing.T) {
	_, err := NewMerger([]Shard{
		{Name: "a", Path: "testdata/merge_a.sqlite"},
		{Name: "gone", Path: "testdata/nosuchshard.sqlite"},
	}, MergeConfig{Table: "kv"})
	require.Error(t, err)
	require.Contains(t, err.Error(), `shard "gone"`)
}

func TestMergeClosed(t *testing.T) {
	m, err := NewMerger([]Shard{
		{Name: "a", Path: "testdata/merge_a.sqlite"},
	}, MergeConfig{Table: "kv"})
	require.NoError(t, err)
	require.NoError(t, m.Close())
	require.NoError(t, m.Close())
	_, _, err = m.Next()
	require.ErrorIs(t, err, ErrMergerClosed)
}

func TestMergeCallback(t *testing.T) {
	var rows [][]string
	err := Merge(
		[]Shard{
			{Name: "a", Path: "testdata/merge_a.sqlite"},
			{Name: "b", Path: "testdata/merge_b.sqlite"},
		},
		MergeConfig{Table: "kv", Columns: []string{"v"}},
		func(r MergedRow) bool {
			rows = append(rows, append([]string{r.Shard}, r.Row.ScanStrings()...))
			return len(rows) == 2
		},
	)
	require.NoError(t, err)
	require.Equal(t, [][]string{
		{"a", "a1"},
		{"a", "a3"},
	}, rows)
}

func TestVerifyShards(t *testing.T) {
	layout, err := VerifyShards([]Shard{
		{Name: "a", Path: "testdata/merge_a.sqlite"},
		{Name: "b", Path: "testdata/merge_b.sqlite"},
	}, MergeConfig{Table: "kv"})
	require.NoError(t, err)
	require.Equal(t, "kv", layout.Table)
	require.Equal(t, []string{"k", "v"}, layout.Columns)

	// the verification catches key order violations
	_, err = VerifyShards([]Shard{
		{Name: "dup", Path: "testdata/merge_dup.sqlite"},
	}, MergeConfig{Table: "kv", Index: "kv_k"})
	require.ErrorIs(t, err, ErrKeyOrder)

	// and conflicts
	_, err = VerifyShards([]Shard{
		{Name: "a", Path: "testdata/merge_conflict_a.sqlite"},
		{Name: "b", Path: "testdata/merge_conflict_b.sqlite"},
	}, MergeConfig{Table: "kv"})
	require.ErrorIs(t, err, ErrBoundaryConflict)
}

func TestMergeReleasesLocks(t *testing.T) {
	// after Close the files must be unlocked and reopenable
	m, err := NewMerger([]Shard{
		{Name: "a", Path: "testdata/merge_a.sqlite"},
		{Name: "b", Path: "testdata/merge_b.sqlite"},
	}, MergeConfig{Table: "kv"})
	require.NoError(t, err)
	require.NoError(t, m.Close())

	again, err := NewMerger([]Shard{
		{Name: "a", Path: "testdata/merge_a.sqlite"},
		{Name: "b", Path: "testdata/merge_b.sqlite"},
	}, MergeConfig{Table: "kv"})
	require.NoError(t, err)
	require.NoError(t, again.Close())
}
