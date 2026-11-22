.PHONY: all clean

ALL:= \
    alter.sqlite \
    empty.sqlite \
    expr.sqlite \
    four.sqlite \
    funkykey.sqlite \
    index.sqlite \
    journal_hot.sqlite \
    journal_persist.sqlite \
    journal_truncate.sqlite \
    magic.sqlite \
    merge_a.sqlite \
    merge_b.sqlite \
    merge_big_a.sqlite \
    merge_big_b.sqlite \
    merge_collate_a.sqlite \
    merge_collate_b.sqlite \
    merge_conflict_a.sqlite \
    merge_conflict_b.sqlite \
    merge_dup.sqlite \
    merge_empty.sqlite \
    merge_idx_a.sqlite \
    merge_idx_b.sqlite \
    merge_layout_bad.sqlite \
    merge_nopk.sqlite \
    merge_wr_a.sqlite \
    merge_wr_b.sqlite \
    merge_wr_idx_a.sqlite \
    merge_wr_idx_b.sqlite \
    music.sqlite \
    notadatabase.sqlite \
    overflow.sqlite \
    prefix.sqlite \
    primarykey.sqlite \
    single.sqlite \
    truncated.sqlite \
    values.sqlite \
    wal.sqlite \
    wal_crashed.sqlite \
    words.sqlite \
    withoutrowid.sqlite \
    zerolength.sqlite \


all: ${ALL}

%.sqlite: %.sh
	bash $<

clean:
	rm -rf *.sqlite
