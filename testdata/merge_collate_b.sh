#!/bin/sh

# second shard with a NOCASE primary key (see merge_collate_a.sh)

DB=merge_collate_b.sqlite

rm -f $DB

sqlite3 --batch $DB <<HERE
CREATE TABLE kv (k TEXT COLLATE NOCASE PRIMARY KEY, v TEXT);
INSERT INTO kv VALUES ('banana', 'b-banana');
INSERT INTO kv VALUES ('date', 'b-date');
INSERT INTO kv VALUES ('Fig', 'b-fig');
HERE
