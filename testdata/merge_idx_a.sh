#!/bin/sh

# first rowid shard merged in DESC index order

DB=merge_idx_a.sqlite

rm -f $DB

sqlite3 --batch $DB <<HERE
CREATE TABLE kv (k TEXT, v TEXT);
CREATE INDEX kv_k ON kv (k DESC);
INSERT INTO kv VALUES ('a', 'a');
INSERT INTO kv VALUES ('c', 'c');
INSERT INTO kv VALUES ('e', 'e');
HERE
