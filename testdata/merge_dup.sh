#!/bin/sh

# shard with a duplicate merge key inside one shard (non-unique index); used to
# test the key order verification

DB=merge_dup.sqlite

rm -f $DB

sqlite3 --batch $DB <<HERE
CREATE TABLE kv (k TEXT, v TEXT);
CREATE INDEX kv_k ON kv (k);
INSERT INTO kv VALUES ('a', 'a');
INSERT INTO kv VALUES ('b', 'b1');
INSERT INTO kv VALUES ('b', 'b2');
INSERT INTO kv VALUES ('c', 'c');
HERE
