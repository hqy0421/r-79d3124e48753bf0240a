#!/bin/sh

# empty shard with the same layout as merge_a.sqlite

DB=merge_empty.sqlite

rm -f $DB

sqlite3 --batch $DB <<HERE
CREATE TABLE kv (k INTEGER PRIMARY KEY, v TEXT);
HERE
