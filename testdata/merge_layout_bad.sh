#!/bin/sh

# shard with a different column layout, used to test verification

DB=merge_layout_bad.sqlite

rm -f $DB

sqlite3 --batch $DB <<HERE
CREATE TABLE kv (k INTEGER PRIMARY KEY, label TEXT);
INSERT INTO kv VALUES (7, 'bad');
HERE
