#!/bin/sh

# first shard with enough rows to span multiple btree pages (see merge_big_b.sh)

DB=merge_big_a.sqlite

rm -f $DB

(
    echo "CREATE TABLE kv (k INTEGER PRIMARY KEY, v TEXT);"
    echo "BEGIN;"
    i=0
    while [ $i -lt 500 ]; do
        echo "INSERT INTO kv VALUES ($((i*2)), 'a-$i');"
        i=$((i+1))
    done
    echo "COMMIT;"
) | sqlite3 --batch $DB
