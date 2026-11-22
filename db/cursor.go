// Pull based iteration over tables and indexes.
//
// The callback based Scan() methods can not be paused and resumed, which makes
// them unsuitable for merging several databases into one ordered stream. The
// cursors in this file are the pull based equivalent: every call to Next()
// returns the next record, and only the state of the btree walk is kept in
// memory. No records are buffered.

package db

import (
	"errors"
)

// TableCursor iterates over a table in database order (rowid order).
//
// The cursor keeps a pointer to the database; the database must stay open for
// the lifetime of the cursor. If you use locks, hold the read lock while you
// use the cursor.
type TableCursor struct {
	db    *Database
	stack []tableCursorFrame
	done  bool
}

type tableCursorFrame struct {
	node tableBtree
	idx  int
}

// Cursor returns a pull based iterator over the table. It is the cursor
// equivalent of Table.Scan().
//
// The returned cursor must be used with the same locking rules as Table.Scan():
// hold a read lock on the database, and don't use the cursor after closing the
// database.
func (t *Table) Cursor() (*TableCursor, error) {
	root, err := t.db.openTable(t.root)
	if err != nil {
		return nil, err
	}
	return &TableCursor{
		db:    t.db,
		stack: []tableCursorFrame{{node: root}},
	}, nil
}

// Next returns the next row. The boolean is false when the cursor is
// exhausted. See Table.Scan() for the meaning of the rowid and record.
func (c *TableCursor) Next() (int64, Record, bool, error) {
	if c.done {
		return 0, nil, false, nil
	}
	for len(c.stack) > 0 {
		f := &c.stack[len(c.stack)-1]
		switch node := f.node.(type) {
		case *tableLeaf:
			if f.idx >= len(node.cells) {
				c.stack = c.stack[:len(c.stack)-1]
				continue
			}
			cell := node.cells[f.idx]
			f.idx++
			rec, err := c.record(cell.payload)
			if err != nil {
				c.done = true
				return 0, nil, false, err
			}
			return cell.left, rec, true, nil

		case *tableInterior:
			if f.idx < len(node.cells) {
				page := node.cells[f.idx].left
				f.idx++
				if err := c.push(page); err != nil {
					c.done = true
					return 0, nil, false, err
				}
				continue
			}
			if f.idx == len(node.cells) {
				// the rightmost child is visited last
				f.idx++
				if err := c.push(node.rightmost); err != nil {
					c.done = true
					return 0, nil, false, err
				}
				continue
			}
			c.stack = c.stack[:len(c.stack)-1]

		default:
			c.done = true
			return 0, nil, false, errors.New("unsupported table page")
		}
	}
	c.done = true
	return 0, nil, false, nil
}

func (c *TableCursor) push(page int) error {
	if len(c.stack) >= maxRecursion {
		return ErrRecursion
	}
	node, err := c.db.openTable(page)
	if err != nil {
		return err
	}
	c.stack = append(c.stack, tableCursorFrame{node: node})
	return nil
}

func (c *TableCursor) record(pl cellPayload) (Record, error) {
	full, err := addOverflow(c.db, pl)
	if err != nil {
		return nil, err
	}
	return parseRecord(full)
}

// IndexCursor iterates over an index, or over a `WITHOUT ROWID` table, in
// index order.
//
// The cursor keeps a pointer to the database; the database must stay open for
// the lifetime of the cursor. If you use locks, hold the read lock while you
// use the cursor.
type IndexCursor struct {
	db    *Database
	stack []indexCursorFrame
	done  bool
}

type indexCursorFrame struct {
	node    indexBtree
	idx     int
	pending *cellPayload // index interior cells are records too, returned after the child
}

// Cursor returns a pull based iterator over the index. It is the cursor
// equivalent of Index.Scan().
//
// The returned cursor must be used with the same locking rules as Index.Scan():
// hold a read lock on the database, and don't use the cursor after closing the
// database.
func (in *Index) Cursor() (*IndexCursor, error) {
	root, err := in.db.openIndex(in.root)
	if err != nil {
		return nil, err
	}
	return &IndexCursor{
		db:    in.db,
		stack: []indexCursorFrame{{node: root}},
	}, nil
}

// Next returns the next record, in index order. The boolean is false when the
// cursor is exhausted. See Index.Scan() for the meaning of the record.
func (c *IndexCursor) Next() (Record, bool, error) {
	if c.done {
		return nil, false, nil
	}
	for len(c.stack) > 0 {
		f := &c.stack[len(c.stack)-1]

		// An interior cell is returned after the child to its left has been
		// fully iterated.
		if f.pending != nil {
			pl := *f.pending
			f.pending = nil
			rec, err := c.record(pl)
			if err != nil {
				c.done = true
				return nil, false, err
			}
			return rec, true, nil
		}

		switch node := f.node.(type) {
		case *indexLeaf:
			if f.idx >= len(node.cells) {
				c.stack = c.stack[:len(c.stack)-1]
				continue
			}
			pl := node.cells[f.idx]
			f.idx++
			rec, err := c.record(pl)
			if err != nil {
				c.done = true
				return nil, false, err
			}
			return rec, true, nil

		case *indexInterior:
			if f.idx < len(node.cells) {
				cell := node.cells[f.idx]
				f.idx++
				f.pending = &cell.payload
				if err := c.push(cell.left); err != nil {
					c.done = true
					return nil, false, err
				}
				continue
			}
			if f.idx == len(node.cells) {
				f.idx++
				if err := c.push(node.rightmost); err != nil {
					c.done = true
					return nil, false, err
				}
				continue
			}
			c.stack = c.stack[:len(c.stack)-1]

		default:
			c.done = true
			return nil, false, errors.New("unsupported index page")
		}
	}
	c.done = true
	return nil, false, nil
}

func (c *IndexCursor) push(page int) error {
	if len(c.stack) >= maxRecursion {
		return ErrRecursion
	}
	node, err := c.db.openIndex(page)
	if err != nil {
		return err
	}
	c.stack = append(c.stack, indexCursorFrame{node: node})
	return nil
}

func (c *IndexCursor) record(pl cellPayload) (Record, error) {
	full, err := addOverflow(c.db, pl)
	if err != nil {
		return nil, err
	}
	return parseRecord(full)
}
