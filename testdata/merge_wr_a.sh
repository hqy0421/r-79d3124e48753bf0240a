#!/bin/sh

# first WITHOUT ROWID shard, composite text/integer primary key

DB=merge_wr_a.sqlite

rm -f $DB

sqlite3 --batch $DB <<HERE
CREATE TABLE kv (k TEXT, n INTEGER, v TEXT, PRIMARY KEY (k, n)) WITHOUT ROWID;
INSERT INTO kv VALUES ('a', 1, 'a1');
INSERT INTO kv VALUES ('a', 3, 'a3');
INSERT INTO kv VALUES ('b', 2, 'b2');
HERE
