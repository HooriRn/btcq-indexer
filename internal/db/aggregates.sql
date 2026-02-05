-- version 1

DROP SCHEMA IF EXISTS btcq_indexer_agg CASCADE;
CREATE SCHEMA btcq_indexer_agg;

CREATE TABLE btcq_indexer_agg.watermarks (
    materialized_table varchar PRIMARY KEY,
    watermark bigint NOT NULL
);

CREATE FUNCTION btcq_indexer_agg.watermark(t varchar) RETURNS bigint
LANGUAGE SQL STABLE AS $$
    SELECT watermark FROM btcq_indexer_agg.watermarks
    WHERE materialized_table = t;
$$;

CREATE PROCEDURE btcq_indexer_agg.refresh_watermarked_view(t varchar, w_new bigint)
LANGUAGE plpgsql AS $BODY$
DECLARE
    w_old bigint;
BEGIN
    SELECT watermark FROM btcq_indexer_agg.watermarks WHERE materialized_table = t
        FOR UPDATE INTO w_old;
    IF w_new <= w_old THEN
        RAISE WARNING 'Updating % into past: % -> %', t, w_old, w_new;
        RETURN;
    END IF;
    EXECUTE format($$
        INSERT INTO btcq_indexer_agg.%1$I_materialized
        SELECT * from btcq_indexer_agg.%1$I
            WHERE $1 <= block_timestamp AND block_timestamp < $2
    $$, t) USING w_old, w_new;
    UPDATE btcq_indexer_agg.watermarks SET watermark = w_new WHERE materialized_table = t;
END
$BODY$;
