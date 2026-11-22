package sqlittle

import (
	"errors"
	"fmt"
	"strings"

	sdb "github.com/alicebob/sqlittle/db"
	"github.com/alicebob/sqlittle/sql"
)

// Shard describes one database file which holds a part of a keyspace.
//
// A keyspace can be split into several SQLite files ("shards"), for example
// by key range. A Merger reads those files as one ordered stream, without
// loading all rows in memory.
type Shard struct {
	// Name identifies the shard. It is passed back with every merged row so
	// the origin of a row stays visible. Names must be unique.
	Name string
	// Path is the filename of the SQLite database.
	Path string
	// Priority decides which row wins a boundary key conflict when
	// ConflictPriority is used. A higher number wins. When two shards have
	// the same priority the shard which was registered first (lowest index in
	// the Shard slice) wins.
	Priority int
}

// ConflictPolicy says what to do when the same key is found in more than one
// shard.
type ConflictPolicy int

const (
	// ConflictError reports an error when a key is present in several shards.
	// This is the default (zero) value.
	ConflictError ConflictPolicy = iota
	// ConflictPriority resolves a key which is present in several shards by
	// picking the row of the shard with the highest Shard.Priority. Ties are
	// won by the earliest registered shard.
	ConflictPriority
)

// MergeConfig describes which table, key, and columns a Merger uses.
type MergeConfig struct {
	// Table is the name of the table in every shard. Required.
	Table string
	// Index is the optional name of the index which defines the merge order
	// and the key. When empty the primary key order is used: the primary key
	// of a WITHOUT ROWID table, the integer primary key (== rowid) of a rowid
	// table, the primary key index of a rowid table with a non-integer primary
	// key, or the rowid for a rowid table without a primary key.
	Index string
	// Columns are the columns of the returned rows, in the order you want
	// them. When empty all columns of the table are returned, in table
	// definition order.
	Columns []string
	// Conflict selects the boundary key conflict behavior.
	Conflict ConflictPolicy
}

// Layout is the (verified) shape shared by all shards.
type Layout struct {
	Table string
	// Columns are the column names, in table definition order.
	Columns []string
	// Types are the declared column types, parallel to Columns. Types are
	// reported for information and are not required to be identical between
	// shards.
	Types []string
	// KeyColumns are the columns which form the merge key (an expression
	// column is reported as its expression text).
	KeyColumns []string
	// KeyCollations and KeyDesc are parallel to KeyColumns.
	KeyCollations []string
	KeyDesc       []bool
	WithoutRowid  bool
}

// MergedRow is one row of a merged stream.
//
// Key and Row are only valid while the Merger is open: some values (BLOBs)
// can point into the database files, just like the Rows from the select
// functions. Use Row.Scan() while you have the row.
type MergedRow struct {
	// Key is the shared key of this row, with the values as stored in the
	// database.
	Key Key
	// Row holds the requested columns.
	Row Row
	// Shard is the Shard.Name of the database this row comes from.
	Shard string
	// ShardIndex is the index in the Shard slice this row comes from.
	ShardIndex int
}

var (
	// ErrKeyOrder is returned when a shard is not strictly ordered by the
	// merge key, or when a key is not unique within a shard.
	ErrKeyOrder = errors.New("shard key order violation")
	// ErrBoundaryConflict is returned when the same key is in several shards
	// and the ConflictPolicy is ConflictError.
	ErrBoundaryConflict = errors.New("key present in multiple shards")
	// ErrShardLayout is returned when the column layout or key definition of a
	// shard differs from the first shard.
	ErrShardLayout = errors.New("shard layout mismatch")
	// ErrMergerClosed is returned when a closed Merger is used.
	ErrMergerClosed = errors.New("merger is closed")
)

// keyLayoutCol is the part of an index column which has to match between
// shards for the merge to make sense.
type keyLayoutCol struct {
	column     string
	expression string
	collate    string
	desc       bool
}

func keyLayout(cols []sdb.IndexColumn) []keyLayoutCol {
	res := make([]keyLayoutCol, len(cols))
	for i, c := range cols {
		res[i] = keyLayoutCol{
			column:     strings.ToLower(c.Column),
			expression: c.Expression,
			collate:    strings.ToLower(c.Collate),
			desc:       c.SortOrder == sql.Desc,
		}
	}
	return res
}

func sameKeyLayout(a, b []keyLayoutCol) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func keyColumnNames(cols []sdb.IndexColumn) []string {
	res := make([]string, len(cols))
	for i, c := range cols {
		if c.Column != "" {
			res[i] = c.Column
		} else {
			res[i] = c.Expression
		}
	}
	return res
}

// shardState is the per-shard cursor and the one row which is currently
// pending in the merge.
type shardState struct {
	index  int
	shard  Shard
	db     *DB
	locked bool
	ci     []columnIndex
	next   func() (Key, sdb.Record, int64, bool, error)

	valid   bool
	head    Key
	headRow Row
	lastKey Key
	hasLast bool
}

// Merger merges several shards into one stream, ordered by a shared key.
//
// NewMerger verifies that every shard has the table, and that the column
// layout and key definition of the shards match. While rows are read the
// order of every shard is verified: keys must be strictly increasing (in the
// order of the key definition) within a shard.
//
// The merger keeps one pending row per shard, plus the state of the btree
// walks, so memory use does not grow with the number of rows.
//
// A Merger holds a read lock on all shards until Close() is called.
type Merger struct {
	cfg     MergeConfig
	layout  Layout
	keyCols []sdb.IndexColumn
	shards  []*shardState
	closed  bool
	err     error
}

// NewMerger opens and read-locks the shards and prepares the merge.
//
// The schema of every shard is verified against the first shard: the table
// must exist everywhere, the columns must match (in name and order), and the
// merge key definition (columns, collation, direction, and without-rowid
// status) must match. A mismatch is reported as ErrShardLayout.
//
// Call Close() when done. On error nothing is left open or locked.
func NewMerger(shards []Shard, cfg MergeConfig) (*Merger, error) {
	if len(shards) == 0 {
		return nil, errors.New("no shards given")
	}
	if cfg.Table == "" {
		return nil, errors.New("MergeConfig.Table is required")
	}
	switch cfg.Conflict {
	case ConflictError, ConflictPriority:
	default:
		return nil, fmt.Errorf("unknown conflict policy: %d", cfg.Conflict)
	}

	names := make(map[string]bool, len(shards))
	paths := make(map[string]bool, len(shards))
	for _, s := range shards {
		if s.Name == "" {
			return nil, fmt.Errorf("shard with empty name (path %q)", s.Path)
		}
		if s.Path == "" {
			return nil, fmt.Errorf("shard %q has an empty path", s.Name)
		}
		if names[s.Name] {
			return nil, fmt.Errorf("duplicate shard name: %q", s.Name)
		}
		names[s.Name] = true
		// The same file twice would make every key a boundary conflict, and
		// the two read locks would fight. Check the path as given; aliases via
		// symlinks or relative paths are not resolved.
		if paths[s.Path] {
			return nil, fmt.Errorf("duplicate shard path: %q", s.Path)
		}
		paths[s.Path] = true
	}

	m := &Merger{cfg: cfg}
	if err := m.open(shards); err != nil {
		m.close()
		return nil, err
	}
	return m, nil
}

func (m *Merger) open(shards []Shard) error {
	// open and lock all files first, so a short read lock conflict is
	// reported before any schema work is done
	for i, sh := range shards {
		db, err := Open(sh.Path)
		if err != nil {
			return fmt.Errorf("shard %q: %w", sh.Name, err)
		}
		st := &shardState{index: i, shard: sh, db: db}
		m.shards = append(m.shards, st)
		if err := db.db.RLock(); err != nil {
			return fmt.Errorf("shard %q: lock: %w", sh.Name, err)
		}
		st.locked = true
	}

	var firstColumns []string
	var firstKey []keyLayoutCol
	for _, st := range m.shards {
		s, err := st.db.db.Schema(m.cfg.Table)
		if err != nil {
			return fmt.Errorf("shard %q: %w", st.shard.Name, err)
		}

		columns := make([]string, len(s.Columns))
		for i, c := range s.Columns {
			columns[i] = c.Column
		}
		if firstColumns == nil {
			firstColumns = columns
			types := make([]string, len(s.Columns))
			for i, c := range s.Columns {
				types[i] = c.Type
			}
			m.layout = Layout{
				Table:        s.Table,
				Columns:      columns,
				Types:        types,
				WithoutRowid: s.WithoutRowid,
			}
		} else {
			if s.WithoutRowid != m.layout.WithoutRowid {
				return fmt.Errorf("shard %q: %w: WITHOUT ROWID is %t, first shard has %t",
					st.shard.Name, ErrShardLayout, s.WithoutRowid, m.layout.WithoutRowid)
			}
			if err := sameColumns(firstColumns, columns); err != nil {
				return fmt.Errorf("shard %q: %w", st.shard.Name, err)
			}
		}

		next, keyCols, err := shardKeySource(st.db, s, m.cfg.Index)
		if err != nil {
			return fmt.Errorf("shard %q: %w", st.shard.Name, err)
		}
		st.next = next

		kl := keyLayout(keyCols)
		if firstKey == nil {
			firstKey = kl
			m.keyCols = keyCols
			m.layout.KeyColumns = keyColumnNames(keyCols)
			m.layout.KeyCollations = make([]string, len(keyCols))
			m.layout.KeyDesc = make([]bool, len(keyCols))
			for i, c := range keyCols {
				m.layout.KeyCollations[i] = strings.ToLower(c.Collate)
				m.layout.KeyDesc[i] = c.SortOrder == sql.Desc
			}
		} else if !sameKeyLayout(firstKey, kl) {
			return fmt.Errorf("shard %q: %w: key layout %s does not match %s",
				st.shard.Name, ErrShardLayout,
				describeKeyLayout(kl), describeKeyLayout(firstKey))
		}
		st.ci, err = outputColumns(s, m.cfg.Columns)
		if err != nil {
			return fmt.Errorf("shard %q: %w", st.shard.Name, err)
		}
	}

	for _, st := range m.shards {
		if err := st.advance(m); err != nil {
			return err
		}
	}
	return nil
}

func sameColumns(a, b []string) error {
	if len(a) != len(b) {
		return fmt.Errorf("%w: have %d columns, first shard has %d", ErrShardLayout, len(b), len(a))
	}
	for i := range a {
		if !strings.EqualFold(a[i], b[i]) {
			return fmt.Errorf("%w: column %d is %q, first shard has %q", ErrShardLayout, i+1, b[i], a[i])
		}
	}
	return nil
}

func describeKeyLayout(kl []keyLayoutCol) string {
	parts := make([]string, len(kl))
	for i, c := range kl {
		n := c.column
		if n == "" {
			n = c.expression
		}
		if c.collate != "" {
			n += " COLLATE " + c.collate
		}
		if c.desc {
			n += " DESC"
		}
		parts[i] = n
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

func outputColumns(s *sdb.Schema, columns []string) ([]columnIndex, error) {
	if len(columns) == 0 {
		columns = make([]string, len(s.Columns))
		for i, c := range s.Columns {
			columns[i] = c.Column
		}
	}
	if s.WithoutRowid {
		return toColumnIndexNonRowid(s, columns)
	}
	return toColumnIndexRowid(s, columns)
}

// shardKeySource gives the pull function and the key definition for one
// shard. The pull function returns the key, the full table record, the rowid
// (0 for WITHOUT ROWID tables), and whether a row was available.
func shardKeySource(db *DB, s *sdb.Schema, index string) (
	func() (Key, sdb.Record, int64, bool, error),
	[]sdb.IndexColumn,
	error,
) {
	if index != "" {
		ind := s.NamedIndex(index)
		if ind == nil {
			return nil, nil, fmt.Errorf("no such index: %q", index)
		}
		return indexKeySource(db, s, ind)
	}

	switch {
	case s.WithoutRowid:
		return nonRowidPKSource(db, s)
	case s.RowidPK:
		return rowidPKSource(db, s)
	case s.PrimaryKey != "":
		ind := s.NamedIndex(s.PrimaryKey)
		if ind == nil {
			return nil, nil, fmt.Errorf("primary key index %q not found", s.PrimaryKey)
		}
		return indexKeySource(db, s, ind)
	default:
		return rowidSource(db, s)
	}
}

func rowidPKSource(db *DB, s *sdb.Schema) (
	func() (Key, sdb.Record, int64, bool, error), []sdb.IndexColumn, error,
) {
	name := "rowid"
	for _, c := range s.Columns {
		if c.Rowid {
			name = c.Column
			break
		}
	}
	next, err := tableRowidSource(db, s)
	if err != nil {
		return nil, nil, err
	}
	return next, []sdb.IndexColumn{{Column: name, SortOrder: sql.Asc}}, nil
}

func rowidSource(db *DB, s *sdb.Schema) (
	func() (Key, sdb.Record, int64, bool, error), []sdb.IndexColumn, error,
) {
	next, err := tableRowidSource(db, s)
	if err != nil {
		return nil, nil, err
	}
	return next, []sdb.IndexColumn{{Column: "rowid", SortOrder: sql.Asc}}, nil
}

func tableRowidSource(db *DB, s *sdb.Schema) (
	func() (Key, sdb.Record, int64, bool, error), error,
) {
	tab, err := db.db.Table(s.Table)
	if err != nil {
		return nil, err
	}
	c, err := tab.Cursor()
	if err != nil {
		return nil, err
	}
	return func() (Key, sdb.Record, int64, bool, error) {
		rowid, rec, ok, err := c.Next()
		if err != nil || !ok {
			return nil, nil, 0, false, err
		}
		return Key{rowid}, rec, rowid, true, nil
	}, nil
}

func nonRowidPKSource(db *DB, s *sdb.Schema) (
	func() (Key, sdb.Record, int64, bool, error), []sdb.IndexColumn, error,
) {
	if len(s.PK) == 0 {
		return nil, nil, errors.New("WITHOUT ROWID table has no primary key")
	}
	stored := columnStoreOrder(s)
	positions := make([]int, len(s.PK))
	for i, c := range s.PK {
		n := s.Column(c.Column)
		if n < 0 {
			return nil, nil, fmt.Errorf("primary key column %q not found", c.Column)
		}
		positions[i] = stored[n]
	}

	tab, err := db.db.NonRowidTable(s.Table)
	if err != nil {
		return nil, nil, err
	}
	c, err := tab.Cursor()
	if err != nil {
		return nil, nil, err
	}
	next := func() (Key, sdb.Record, int64, bool, error) {
		rec, ok, err := c.Next()
		if err != nil || !ok {
			return nil, nil, 0, false, err
		}
		key := make(Key, len(positions))
		for i, p := range positions {
			if p >= len(rec) {
				return nil, nil, 0, false, fmt.Errorf("%w: primary key column %q missing from record", ErrKeyOrder, s.PK[i].Column)
			}
			key[i] = rec[p]
		}
		return key, rec, 0, true, nil
	}
	return next, keyColumns(s, s.PK), nil
}

// keyColumns returns a copy of cols with the collation filled in from the
// column definitions. On-disk order follows the column collation, which the
// schema does not always repeat in a primary key definition.
func keyColumns(s *sdb.Schema, cols []sdb.IndexColumn) []sdb.IndexColumn {
	res := make([]sdb.IndexColumn, len(cols))
	copy(res, cols)
	for i, c := range res {
		if c.Column == "" || c.Collate != "" {
			continue
		}
		if n := s.Column(c.Column); n >= 0 {
			res[i].Collate = s.Columns[n].Collate
		}
	}
	return res
}

func indexKeySource(db *DB, s *sdb.Schema, ind *sdb.SchemaIndex) (
	func() (Key, sdb.Record, int64, bool, error), []sdb.IndexColumn, error,
) {
	nKey := len(ind.Columns)
	keyCols := keyColumns(s, ind.Columns)

	idx, err := db.db.Index(ind.Index)
	if err != nil {
		return nil, nil, err
	}
	c, err := idx.Cursor()
	if err != nil {
		return nil, nil, err
	}

	if s.WithoutRowid {
		tab, err := db.db.NonRowidTable(s.Table)
		if err != nil {
			return nil, nil, err
		}
		// pkColumns appends primary key columns which are not in the index to
		// the index definition; the record layout follows that.
		pkPos := pkColumns(s, ind)
		pkCols := keyColumns(s, s.PK)
		pkKey, err := asDbKey(make(Key, len(pkCols)), pkCols)
		if err != nil {
			return nil, nil, err
		}
		next := func() (Key, sdb.Record, int64, bool, error) {
			rec, ok, err := c.Next()
			if err != nil || !ok {
				return nil, nil, 0, false, err
			}
			key := make(Key, nKey)
			copy(key, rec)
			for i, p := range pkPos {
				if p >= len(rec) {
					return nil, nil, 0, false, fmt.Errorf("%w: primary key column %q missing from index %q", ErrKeyOrder, s.PK[i].Column, ind.Index)
				}
				pkKey[i].V = rec[p]
			}
			var found sdb.Record
			if err := tab.ScanEq(pkKey, func(r sdb.Record) bool {
				found = r
				return true
			}); err != nil {
				return nil, nil, 0, false, err
			}
			if found == nil {
				return nil, nil, 0, false, fmt.Errorf("index %q points to a missing row", ind.Index)
			}
			return key, found, 0, true, nil
		}
		return next, keyCols, nil
	}

	tab, err := db.db.Table(s.Table)
	if err != nil {
		return nil, nil, err
	}
	next := func() (Key, sdb.Record, int64, bool, error) {
		rec, ok, err := c.Next()
		if err != nil || !ok {
			return nil, nil, 0, false, err
		}
		rowid, _, err := sdb.ChompRowid(rec)
		if err != nil {
			return nil, nil, 0, false, err
		}
		tabRec, err := tab.Rowid(rowid)
		if err != nil {
			return nil, nil, 0, false, err
		}
		if tabRec == nil {
			return nil, nil, 0, false, fmt.Errorf("index %q points to a missing rowid %d", ind.Index, rowid)
		}
		key := make(Key, nKey)
		copy(key, rec)
		return key, tabRec, rowid, true, nil
	}
	return next, keyCols, nil
}

// compareKeys compares two keys in the order of the merge key definition.
// Shorter keys compare as if padded with NULLs, which matches SQLite's index
// comparison. All key sources return keys with the full key length, though.
func (m *Merger) compareKeys(a, b Key) int {
	for i, kc := range m.keyCols {
		var av, bv interface{}
		if i < len(a) {
			av = a[i]
		}
		if i < len(b) {
			bv = b[i]
		}
		c := sdb.CompareValues(av, bv, strings.ToLower(kc.Collate))
		if c == 0 {
			continue
		}
		if kc.SortOrder == sql.Desc {
			return -c
		}
		return c
	}
	return 0
}

// advance loads the next row of a shard, and verifies it is ordered after the
// previous row of that same shard.
func (st *shardState) advance(m *Merger) error {
	key, rec, rowid, ok, err := st.next()
	if err != nil {
		return fmt.Errorf("shard %q: %w", st.shard.Name, err)
	}
	if !ok {
		st.valid = false
		st.head = nil
		st.headRow = nil
		return nil
	}
	if st.hasLast {
		if c := m.compareKeys(key, st.lastKey); c <= 0 {
			return fmt.Errorf("%w: shard %q: key %s is not after previous key %s",
				ErrKeyOrder, st.shard.Name, formatKey(key), formatKey(st.lastKey))
		}
	}
	st.lastKey = key
	st.hasLast = true
	st.head = key
	st.headRow = toRow(rowid, st.ci, rec)
	st.valid = true
	return nil
}

func formatKey(k Key) string {
	parts := make([]string, len(k))
	for i, v := range k {
		parts[i] = fmt.Sprintf("%v", v)
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

// Next returns the next row of the merged stream. The boolean is false when
// all shards are exhausted. After an error Next keeps returning that error.
func (m *Merger) Next() (MergedRow, bool, error) {
	if m.closed {
		return MergedRow{}, false, ErrMergerClosed
	}
	if m.err != nil {
		return MergedRow{}, false, m.err
	}

	var minKey Key
	var winners []*shardState
	for _, st := range m.shards {
		if !st.valid {
			continue
		}
		if minKey == nil {
			minKey = st.head
			winners = []*shardState{st}
			continue
		}
		switch c := m.compareKeys(st.head, minKey); {
		case c < 0:
			minKey = st.head
			winners = []*shardState{st}
		case c == 0:
			winners = append(winners, st)
		}
	}
	if len(winners) == 0 {
		return MergedRow{}, false, nil
	}

	winner := winners[0]
	if len(winners) > 1 {
		switch m.cfg.Conflict {
		case ConflictPriority:
			for _, st := range winners[1:] {
				if st.shard.Priority > winner.shard.Priority {
					winner = st
				}
			}
		default: // ConflictError
			names := make([]string, len(winners))
			for i, st := range winners {
				names[i] = fmt.Sprintf("%q", st.shard.Name)
			}
			m.err = fmt.Errorf("%w: key %s in shards %s",
				ErrBoundaryConflict, formatKey(minKey), strings.Join(names, ", "))
			return MergedRow{}, false, m.err
		}
	}

	out := MergedRow{
		Key:        minKey,
		Row:        winner.headRow,
		Shard:      winner.shard.Name,
		ShardIndex: winner.index,
	}

	// Advance every shard which had this key, including the losing ones.
	for _, st := range winners {
		if err := st.advance(m); err != nil {
			m.err = err
			return MergedRow{}, false, err
		}
	}
	return out, true, nil
}

// Layout returns the verified layout shared by all shards. The returned
// slices are copies.
func (m *Merger) Layout() Layout {
	l := m.layout
	l.Columns = append([]string(nil), m.layout.Columns...)
	l.Types = append([]string(nil), m.layout.Types...)
	l.KeyColumns = append([]string(nil), m.layout.KeyColumns...)
	l.KeyCollations = append([]string(nil), m.layout.KeyCollations...)
	l.KeyDesc = append([]bool(nil), m.layout.KeyDesc...)
	return l
}

// Close releases the read locks and closes all shard files. It is safe to
// call Close more than once.
func (m *Merger) Close() error {
	if m.closed {
		return nil
	}
	m.closed = true
	return m.close()
}

func (m *Merger) close() error {
	var firstErr error
	for _, st := range m.shards {
		if st.locked {
			if err := st.db.db.RUnlock(); err != nil && firstErr == nil {
				firstErr = err
			}
			st.locked = false
		}
		if st.db != nil {
			if err := st.db.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// Merge is a callback based wrapper around Merger. The callback is called for
// every row, in merge order. Returning true stops the iteration. Each Row is
// only valid during the callback, like the Rows of Select. The merger is
// closed before Merge returns.
func Merge(shards []Shard, cfg MergeConfig, cb func(MergedRow) bool) error {
	m, err := NewMerger(shards, cfg)
	if err != nil {
		return err
	}
	for {
		row, ok, err := m.Next()
		if err != nil {
			m.Close()
			return err
		}
		if !ok {
			break
		}
		if cb(row) {
			break
		}
	}
	return m.Close()
}

// VerifyShards opens the shards and reads the complete merged stream, which
// verifies the column layout, the key layout, and the key order of every
// shard. Boundary key conflicts follow cfg.Conflict. The verified layout is
// returned.
//
// This reads all rows, but does not keep them: memory use is still bounded by
// the number of shards.
func VerifyShards(shards []Shard, cfg MergeConfig) (Layout, error) {
	m, err := NewMerger(shards, cfg)
	if err != nil {
		return Layout{}, err
	}
	defer m.Close()
	for {
		_, ok, err := m.Next()
		if err != nil {
			return m.Layout(), err
		}
		if !ok {
			return m.Layout(), nil
		}
	}
}
