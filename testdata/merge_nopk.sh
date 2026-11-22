#!/bin/sh

# shard without a primary key (merge key is the rowid), used to test key layout
# verification against merge_conflict_a.sqlite

DB=merge_nopk.sqlite

rm -f $DB

sqlite3 --batch $DB <<HERE
CREATE TABLE kv (k TEXT, v TEXT);
INSERT INTO kv VALUES ('n1', 'n1');
INSERT INTO kv VALUES ('n2', 'n2');
HERE
