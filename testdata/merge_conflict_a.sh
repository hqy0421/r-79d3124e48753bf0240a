#!/bin/sh

# shard which overlaps merge_conflict_b.sqlite on key 'm'

DB=merge_conflict_a.sqlite

rm -f $DB

sqlite3 --batch $DB <<HERE
CREATE TABLE kv (k TEXT PRIMARY KEY, v TEXT);
INSERT INTO kv VALUES ('a', 'a');
INSERT INTO kv VALUES ('m', 'from-a');
INSERT INTO kv VALUES ('z', 'z');
HERE
