#!/bin/sh

# second WITHOUT ROWID shard, composite text/integer primary key

DB=merge_wr_b.sqlite

rm -f $DB

sqlite3 --batch $DB <<HERE
CREATE TABLE kv (k TEXT, n INTEGER, v TEXT, PRIMARY KEY (k, n)) WITHOUT ROWID;
INSERT INTO kv VALUES ('b', 5, 'b5');
INSERT INTO kv VALUES ('c', 1, 'c1');
HERE
