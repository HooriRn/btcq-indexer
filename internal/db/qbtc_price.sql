INSERT INTO btcq_indexer_agg.watermarks (materialized_table, watermark)
    VALUES ('qbtc_price', 0);

CREATE TABLE btcq_indexer_agg.qbtc_price (
    qbtc_price_usd DOUBLE PRECISION NOT NULL,
    block_timestamp bigint NOT NULL,
    PRIMARY KEY(block_timestamp)
);

-- TODO(hooriRN): fill with actual price instead of a constant
CREATE PROCEDURE btcq_indexer_agg.update_qbtc_price_interval(t1 bigint, t2 bigint)
LANGUAGE plpgsql AS $BODY$
BEGIN
    INSERT INTO btcq_indexer_agg.qbtc_price AS cb (
        SELECT 
            1 AS qbtc_price_usd,
            timestamp as block_timestamp
        FROM block_log
        WHERE t1 <= timestamp AND timestamp < t2
    )
    ON CONFLICT (block_timestamp) DO UPDATE SET qbtc_price_usd = cb.qbtc_price_usd;
END
$BODY$;

CREATE PROCEDURE btcq_indexer_agg.update_qbtc_price(w_new bigint)
LANGUAGE plpgsql AS $BODY$
DECLARE
    w_old bigint;
BEGIN
    SELECT watermark FROM btcq_indexer_agg.watermarks WHERE materialized_table = 'qbtc_price'
        FOR UPDATE INTO w_old;
    IF w_new <= w_old THEN
        RAISE WARNING 'Updating qbtc prices into past: % -> %', w_old, w_new;
        RETURN;
    END IF;
    CALL btcq_indexer_agg.update_qbtc_price_interval(w_old, w_new);
    UPDATE btcq_indexer_agg.watermarks SET watermark = w_new WHERE materialized_table = 'qbtc_price';
END
$BODY$;
