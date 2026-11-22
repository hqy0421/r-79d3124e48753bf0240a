#!/bin/sh

# first WITHOUT ROWID shard merged via a secondary index (see
# merge_wr_idx_b.sh). Index order is by score, then the primary key.

DB=merge_wr_idx_a.sqlite

rm -f $DB

sqlite3 --batch $DB <<HERE
CREATE TABLE kv (k TEXT, score INTEGER, v TEXT, PRIMARY KEY (k)) WITHOUT ROWID;
CREATE INDEX kv_score ON kv (score);
INSERT INTO kv VALUES ('a', 10, 'a');
INSERT INTO kv VALUES ('c', 30, 'c');
INSERT INTO kv VALUES ('e', 50, 'e');
HERE
