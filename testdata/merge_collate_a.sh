#!/bin/sh

# first shard with a NOCASE primary key; sqlite stores and sorts the key
# case-insensitively (see merge_collate_b.sh)

DB=merge_collate_a.sqlite

rm -f $DB

sqlite3 --batch $DB <<HERE
CREATE TABLE kv (k TEXT COLLATE NOCASE PRIMARY KEY, v TEXT);
INSERT INTO kv VALUES ('apple', 'a-apple');
INSERT INTO kv VALUES ('Cherry', 'a-cherry');
INSERT INTO kv VALUES ('grape', 'a-grape');
HERE
