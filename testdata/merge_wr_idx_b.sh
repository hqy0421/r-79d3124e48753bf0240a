#!/bin/sh

# second WITHOUT ROWID shard merged via a secondary index (see
# merge_wr_idx_a.sh)

DB=merge_wr_idx_b.sqlite

rm -f $DB

sqlite3 --batch $DB <<HERE
CREATE TABLE kv (k TEXT, score INTEGER, v TEXT, PRIMARY KEY (k)) WITHOUT ROWID;
CREATE INDEX kv_score ON kv (score);
INSERT INTO kv VALUES ('b', 20, 'b');
INSERT INTO kv VALUES ('d', 40, 'd');
INSERT INTO kv VALUES ('f', 60, 'f');
HERE
