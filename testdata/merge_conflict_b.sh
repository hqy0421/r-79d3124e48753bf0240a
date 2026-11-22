#!/bin/sh

# shard which overlaps merge_conflict_a.sqlite on key 'm'

DB=merge_conflict_b.sqlite

rm -f $DB

sqlite3 --batch $DB <<HERE
CREATE TABLE kv (k TEXT PRIMARY KEY, v TEXT);
INSERT INTO kv VALUES ('b', 'b');
INSERT INTO kv VALUES ('m', 'from-b');
INSERT INTO kv VALUES ('y', 'y');
HERE
