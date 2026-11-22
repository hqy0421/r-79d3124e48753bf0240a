#!/bin/sh

# second rowid shard merged in DESC index order

DB=merge_idx_b.sqlite

rm -f $DB

sqlite3 --batch $DB <<HERE
CREATE TABLE kv (k TEXT, v TEXT);
CREATE INDEX kv_k ON kv (k DESC);
INSERT INTO kv VALUES ('b', 'b');
INSERT INTO kv VALUES ('d', 'd');
INSERT INTO kv VALUES ('f', 'f');
HERE
