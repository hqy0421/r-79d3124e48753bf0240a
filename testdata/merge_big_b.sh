#!/bin/sh

# second shard with enough rows to span multiple btree pages (see merge_big_a.sh)

DB=merge_big_b.sqlite

rm -f $DB

(
    echo "CREATE TABLE kv (k INTEGER PRIMARY KEY, v TEXT);"
    echo "BEGIN;"
    i=0
    while [ $i -lt 500 ]; do
        echo "INSERT INTO kv VALUES ($((i*2+1)), 'b-$i');"
        i=$((i+1))
    done
    echo "COMMIT;"
) | sqlite3 --batch $DB
