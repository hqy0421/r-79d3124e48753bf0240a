#!/bin/sh

# second rowid shard of a split keyspace (see merge_a.sh)

DB=merge_b.sqlite

rm -f $DB

sqlite3 --batch $DB <<HERE
CREATE TABLE kv (k INTEGER PRIMARY KEY, v TEXT);
INSERT INTO kv VALUES (6, 'b6');
INSERT INTO kv VALUES (8, 'b8');
INSERT INTO kv VALUES (9, 'b9');
HERE
