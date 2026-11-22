#!/bin/sh

# first rowid shard of a split keyspace (see merge_b.sh)

DB=merge_a.sqlite

rm -f $DB

sqlite3 --batch $DB <<HERE
CREATE TABLE kv (k INTEGER PRIMARY KEY, v TEXT);
INSERT INTO kv VALUES (1, 'a1');
INSERT INTO kv VALUES (3, 'a3');
INSERT INTO kv VALUES (5, 'a5');
HERE
