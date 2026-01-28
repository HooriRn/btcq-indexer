-- version 1

DROP SCHEMA IF EXISTS btcq_indexer_agg CASCADE;
CREATE SCHEMA btcq_indexer_agg;

CREATE VIEW btcq_indexer_agg.pending_adds AS
SELECT *
FROM pending_liquidity_events AS p
WHERE pending_type = 'add'
    AND NOT EXISTS(
        -- Filter out pending liquidity which was already added
        SELECT *
        FROM stake_events AS s
        WHERE
            p.rune_addr = s.rune_addr
            AND p.pool = s.pool
            AND p.block_timestamp <= s.block_timestamp)
    AND NOT EXISTS(
        -- Filter out pending liquidity which was withdrawn without adding
        SELECT *
        FROM pending_liquidity_events AS pw
        WHERE
            pw.pending_type = 'withdraw'
            AND p.rune_addr = pw.rune_addr
            AND p.pool = pw.pool
            AND p.block_timestamp <= pw.block_timestamp);

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

-------------------------------------------------------------------------------
-- Thorname
-------------------------------------------------------------------------------

-- TODO(muninn): replace with indexing time materialized table, a full select is 100ms.
CREATE VIEW btcq_indexer_agg.thorname_owner_expiration AS
    SELECT DISTINCT ON (name)
        name,
        owner,
        expire
    FROM thorname_change_events
    ORDER BY name, block_timestamp DESC;

CREATE VIEW btcq_indexer_agg.thorname_last_owner AS
WITH owner_changes AS (
        SELECT 
            name, 
            owner, 
            block_timestamp,
            LAG(owner) OVER (PARTITION BY name ORDER BY block_timestamp) AS previous_owner
        FROM thorname_change_events
    )
    SELECT DISTINCT ON (name)
        name,
        block_timestamp
    FROM owner_changes
    WHERE owner <> previous_owner OR previous_owner IS NULL
    ORDER BY name, block_timestamp DESC;

CREATE VIEW btcq_indexer_agg.thorname_current_state AS
    SELECT DISTINCT ON (name, chain)
        change_events.name,
        change_events.chain,
        change_events.address,
        owner_expiration.owner,
        owner_expiration.expire
    FROM thorname_change_events AS change_events
    JOIN btcq_indexer_agg.thorname_owner_expiration AS owner_expiration
        ON owner_expiration.name = change_events.name
    JOIN btcq_indexer_agg.thorname_last_owner AS last_owner
        ON change_events.name = last_owner.name
    WHERE last_owner.block_timestamp <= change_events.block_timestamp
    ORDER BY name, chain, change_events.block_timestamp DESC;

-------------------------------------------------------------------------------
-- Actions
-------------------------------------------------------------------------------

--
-- Main table and its indices
--

CREATE TABLE btcq_indexer_agg.actions (
    event_id            bigint NOT NULL,
    block_timestamp     bigint NOT NULL,
    action_type         text NOT NULL,
    main_ref            text,
    addresses           text[] NOT NULL,
    transactions        text[] NOT NULL,
    assets              text[] NOT NULL,
    pools               text[],
    ins                 jsonb NOT NULL,
    outs                jsonb NOT NULL,
    fees                jsonb NOT NULL,
    meta                jsonb,
    streaming_meta      jsonb
) WITH (fillfactor = 90);
-- TODO(huginn): should it be a hypertable? Measure both ways and decide!

-- Table constraint
CREATE UNIQUE INDEX swap_idx ON btcq_indexer_agg.actions (main_ref) WHERE action_type IN ('swap', 'trade', 'secure');

CREATE INDEX ON btcq_indexer_agg.actions (event_id DESC);
CREATE INDEX ON btcq_indexer_agg.actions (action_type, event_id DESC);
CREATE INDEX ON btcq_indexer_agg.actions (main_ref, event_id DESC);
CREATE INDEX ON btcq_indexer_agg.actions (block_timestamp DESC);

CREATE INDEX ON btcq_indexer_agg.actions USING gin (addresses);
CREATE INDEX ON btcq_indexer_agg.actions USING gin (transactions);
CREATE INDEX ON btcq_indexer_agg.actions USING gin (assets);
CREATE INDEX ON btcq_indexer_agg.actions USING gin ((meta -> 'affiliateAddress'));
CREATE INDEX ON btcq_indexer_agg.actions USING gin (string_to_array(lower(meta->>'affiliateAddress'), '/'));
CREATE INDEX ON btcq_indexer_agg.actions USING gin ((meta -> 'txType'));

--
-- Functions for actions aggregates
--

CREATE FUNCTION btcq_indexer_agg.out_tx(
    txid text,
    address text,
    height text,
    internal boolean,
    VARIADIC coins coin_rec[]
) RETURNS jsonb
LANGUAGE plpgsql AS $BODY$
DECLARE
    ret jsonb;
BEGIN
    ret := jsonb_build_object('txID', txid, 'address', address, 'coins', coins(VARIADIC coins));
    IF height IS NOT NULL THEN
        ret := ret || jsonb_build_object('height', height);
    END IF;
    IF internal IS NOT NULL THEN
        ret := ret || jsonb_build_object('internal', internal);
    END IF;

    RETURN ret;
END
$BODY$;

--
-- Basic VIEWs that build actions
--

CREATE VIEW btcq_indexer_agg.switch_actions AS
    SELECT
        event_id,
        block_timestamp,
        'switch' AS action_type,
        tx :: text AS main_ref,
        ARRAY[from_addr, to_addr] :: text[] AS addresses,
        non_null_array(tx) AS transactions,
        ARRAY[burn_asset, mint_asset] :: text[] AS assets,
        NULL :: text[] AS pools,
        jsonb_build_array(mktransaction(tx, from_addr, (burn_asset, burn_e8))) AS ins,
        jsonb_build_array(mktransaction(NULL, to_addr, (mint_asset, mint_e8))) AS outs,
        jsonb_build_array() AS fees,
        NULL :: jsonb AS meta
    FROM switch_events;

CREATE VIEW btcq_indexer_agg.refund_actions AS
    SELECT
        event_id,
        block_timestamp,
        'refund' AS action_type,
        tx :: text AS main_ref,
        ARRAY[from_addr, to_addr] :: text[] AS addresses,
        ARRAY[tx] :: text[] AS transactions,
        non_null_array(asset, asset_2nd) AS assets,
        NULL :: text[] AS pools,
        jsonb_build_array(mktransaction(tx, from_addr, (asset, asset_e8))) AS ins,
        jsonb_build_array() AS outs,
        jsonb_build_array() AS fees,
        jsonb_build_object(
            'reason', reason,
            'memo', memo,
            'affiliateFee', CASE
                WHEN SUBSTRING(memo FROM '^(.*?):')::text = ANY('{SWAP,s,=}') THEN
                    SUBSTRING(memo FROM '^(?:=|SWAP|[s]):(?:[^:]*:){4}(\d{1,5}?)(?::|$)')::int
                WHEN SUBSTRING(memo FROM '^(.*?):')::text = ANY('{ADD,a,+}') THEN
                    SUBSTRING(memo FROM '^(?:ADD|[+]|a):(?:[^:]*:){3}(\d{1,5}?)(?::|$)')::int
                ELSE NULL
            END,
            'affiliateAddress', CASE
                WHEN SUBSTRING(memo FROM '^(.*?):')::text = ANY('{SWAP,s,=}') THEN
                    SUBSTRING(memo FROM '^(?:=|SWAP|[s]):(?:[^:]*:){3}([^:]+)')
                WHEN SUBSTRING(memo FROM '^(.*?):')::text = ANY('{ADD,a,+}') THEN
                    SUBSTRING(memo FROM '^(?:ADD|[+]|a):(?:[^:]*:){2}([^:]+)')
                ELSE NULL
            END,
            'txType', _tx_type
            ) AS meta
    FROM refund_events;

CREATE VIEW btcq_indexer_agg.donate_actions AS
    SELECT
        event_id,
        block_timestamp,
        'donate' AS action_type,
        tx :: text AS main_ref,
        ARRAY[from_addr, to_addr] :: text[] AS addresses,
        ARRAY[tx] :: text[] AS transactions,
        CASE WHEN rune_e8 > 0 THEN ARRAY[asset, 'THOR.RUNE']
            ELSE ARRAY[asset] END :: text[] AS assets,
        ARRAY[pool] :: text[] AS pools,
        jsonb_build_array(mktransaction(tx, from_addr, (asset, asset_e8),
            ('THOR.RUNE', rune_e8))) AS ins,
        jsonb_build_array() AS outs,
        jsonb_build_array() AS fees,
        NULL :: jsonb AS meta
    FROM add_events;

CREATE VIEW btcq_indexer_agg.withdraw_actions AS
    SELECT
        event_id,
        block_timestamp,
        'withdraw' AS action_type,
        tx :: text AS main_ref,
        ARRAY[from_addr, to_addr] :: text[] AS addresses,
        ARRAY[tx] :: text[] AS transactions,
        ARRAY[pool] :: text[] AS assets,
        ARRAY[pool] :: text[] AS pools,
        jsonb_build_array(mktransaction(tx, from_addr, (asset, asset_e8))) AS ins,
        jsonb_build_array() AS outs,
        jsonb_build_array() AS fees,
        jsonb_build_object(
            'asymmetry', asymmetry,
            'basisPoints', basis_points,
            'impermanentLossProtection', imp_loss_protection_e8,
            'liquidityUnits', -stake_units,
            'emitAssetE8', emit_asset_e8,
            'emitRuneE8', emit_rune_e8,
            'memo', memo
            ) AS meta
    FROM withdraw_events;

-- calculate the latest price of the asset
CREATE VIEW btcq_indexer_agg.swap_usd_actions AS 
    SELECT
        s.*,
        COALESCE(b.rune_e8::DOUBLE PRECISION / NULLIF(b.asset_e8::DOUBLE PRECISION, 0), 0) * r.rune_price_e8 AS asset_price_usd,
        r.rune_price_e8 AS rune_price_usd
    FROM
        swap_events as s
    LEFT JOIN
        rune_price as r ON s.block_timestamp = r.block_timestamp
    LEFT JOIN LATERAL (
        SELECT asset_e8, rune_e8
        FROM block_pool_depths
        WHERE block_pool_depths.pool = s.pool
        AND block_pool_depths.block_timestamp <= s.block_timestamp
        ORDER BY block_timestamp DESC
        LIMIT 1
    ) AS b ON true;


-- TODO(huginn): use _direction for join
CREATE VIEW btcq_indexer_agg.swap_actions AS
    -- Double swap (same txid in different pools)
        SELECT
        swap_in.event_id,
        swap_in.block_timestamp,
        'swap' AS action_type,
        swap_in.tx :: text AS main_ref,
        ARRAY[swap_in.from_addr, swap_in.to_addr] :: text[] AS addresses,
        ARRAY[swap_in.tx] :: text[] AS transactions,
        ARRAY[swap_in.from_asset, swap_out.to_asset] :: text[] AS assets,
        CASE WHEN swap_in.pool <> swap_out.pool THEN ARRAY[swap_in.pool, swap_out.pool]
            ELSE ARRAY[swap_in.pool] END :: text[] AS pools,
        jsonb_build_array(mktransaction(swap_in.tx, swap_in.from_addr,
            (swap_in.from_asset, swap_in.from_e8))) AS ins,
        jsonb_build_array() AS outs,
        jsonb_build_array() AS fees,
        jsonb_build_object(
            'swapSingle', FALSE,
            'liquidityFee', swap_in.liq_fee_in_rune_e8 + swap_out.liq_fee_in_rune_e8,
            'swapTarget', swap_out.to_e8_min,
            'swapSlip', swap_in.swap_slip_BP + swap_out.swap_slip_BP
                - swap_in.swap_slip_BP*swap_out.swap_slip_BP/10000,
            'memo', swap_in.memo,
            'affiliateFee', CASE
                WHEN SUBSTRING(swap_in.memo FROM '^(.*?):')::text = ANY('{SWAP,s,=}') THEN
                    SUBSTRING(swap_in.memo FROM '^(?:=|SWAP|[s]):(?:[^:]*:){4}(\d{1,5}?)(?::|$)')::int
                WHEN SUBSTRING(swap_in.memo FROM '^(.*?):')::text = ANY('{ADD,a,+}') THEN
                    SUBSTRING(swap_in.memo FROM '^(?:ADD|[+]|a):(?:[^:]*:){3}(\d{1,5}?)(?::|$)')::int
                ELSE NULL
            END,
            'affiliateAddress', CASE
                WHEN SUBSTRING(swap_in.memo FROM '^(.*?):')::text = ANY('{SWAP,s,=}') THEN
                    SUBSTRING(swap_in.memo FROM '^(?:=|SWAP|[s]):(?:[^:]*:){3}([^:]+)')
                WHEN SUBSTRING(swap_in.memo FROM '^(.*?):')::text = ANY('{ADD,a,+}') THEN
                    SUBSTRING(swap_in.memo FROM '^(?:ADD|[+]|a):(?:[^:]*:){2}([^:]+)')
                ELSE NULL
            END,
            'outRuneE8', swap_in.to_e8,
            'txType', swap_in._tx_type,
            'inPriceUSD', swap_in.asset_price_usd,
            'outPriceUSD', swap_out.asset_price_usd
            ) AS meta,
        jsonb_build_object(
            'count', swap_in.streaming_count,
            'quantity', swap_in.streaming_quantity,
            'out_estimation', swap_out.to_e8 * swap_in.streaming_quantity
            ) AS streaming_meta
    FROM btcq_indexer_agg.swap_usd_actions AS swap_in
    INNER JOIN btcq_indexer_agg.swap_usd_actions AS swap_out
    ON swap_in.tx = swap_out.tx AND swap_in.block_timestamp = swap_out.block_timestamp
    WHERE swap_in.from_asset <> swap_out.to_asset AND swap_in.to_e8 = swap_out.from_e8
        AND swap_in.to_asset = 'THOR.RUNE' AND swap_out.from_asset = 'THOR.RUNE'
        AND swap_in.memo != 'noop'
    UNION ALL
    -- Single swap (unique txid)
    SELECT
        event_id,
        block_timestamp,
        'swap' AS action_type,
        tx :: text AS main_ref,
        ARRAY[from_addr, to_addr] :: text[] AS addresses,
        ARRAY[tx] :: text[] AS transactions,
        ARRAY[from_asset, to_asset] :: text[] AS assets,
        ARRAY[pool] :: text[] AS pools,
        jsonb_build_array(mktransaction(tx, from_addr, (from_asset, from_e8))) AS ins,
        jsonb_build_array() AS outs,
        jsonb_build_array() AS fees,
        jsonb_build_object(
            'swapSingle', TRUE,
            'liquidityFee', liq_fee_in_rune_e8,
            'swapTarget', to_e8_min,
            'swapSlip', swap_slip_bp,
            'memo', memo,
            'affiliateFee', CASE
                WHEN SUBSTRING(memo FROM '^(.*?):')::text = ANY('{SWAP,s,=}') THEN
                    SUBSTRING(memo FROM '^(?:=|SWAP|[s]):(?:[^:]*:){4}(\d{1,5}?)(?::|$)')::int
                WHEN SUBSTRING(memo FROM '^(.*?):')::text = ANY('{ADD,a,+}') THEN
                    SUBSTRING(memo FROM '^(?:ADD|[+]|a):(?:[^:]*:){3}(\d{1,5}?)(?::|$)')::int
                ELSE NULL
            END,
            'affiliateAddress', CASE
                WHEN SUBSTRING(memo FROM '^(.*?):')::text = ANY('{SWAP,s,=}') THEN
                    SUBSTRING(memo FROM '^(?:=|SWAP|[s]):(?:[^:]*:){3}([^:]+)')
                WHEN SUBSTRING(memo FROM '^(.*?):')::text = ANY('{ADD,a,+}') THEN
                    SUBSTRING(memo FROM '^(?:ADD|[+]|a):(?:[^:]*:){2}([^:]+)')
                ELSE NULL
            END,
            'txType', _tx_type,
            'inPriceUSD', CASE
                WHEN _direction%2 = 0 THEN
                    rune_price_usd
                ELSE
                    asset_price_usd
                END,
            'outPriceUSD', CASE
                WHEN _direction%2 = 0 THEN
                    asset_price_usd
                ELSE
                    rune_price_usd
                END
            ) AS meta,
        jsonb_build_object(
            'count', streaming_count,
            'quantity', streaming_quantity,
            'out_estimation', to_e8 * streaming_quantity
            ) AS streaming_meta
    FROM btcq_indexer_agg.swap_usd_actions AS single_swaps
    WHERE NOT EXISTS (
        SELECT tx FROM swap_events
        WHERE block_timestamp = single_swaps.block_timestamp AND tx = single_swaps.tx
            AND (from_e8 = single_swaps.to_e8 OR to_e8 = single_swaps.from_e8)
    );

CREATE VIEW btcq_indexer_agg.addliquidity_actions AS
    SELECT
        event_id,
        block_timestamp,
        'addLiquidity' AS action_type,
        NULL :: text AS main_ref,
        non_null_array(rune_addr, asset_addr) AS addresses,
        non_null_array(rune_tx, asset_tx) AS transactions,
        ARRAY[pool] :: text[] AS assets,
        ARRAY[pool] :: text[] AS pools,
        transaction_list(
            mktransaction(rune_tx, rune_addr, ('THOR.RUNE', rune_e8)),
            mktransaction(asset_tx, asset_addr, (pool, asset_e8))
            ) AS ins,
        jsonb_build_array() AS outs,
        jsonb_build_array() AS fees,
        jsonb_build_object(
            'status', 'success',
            'liquidityUnits', stake_units,
            'memo', memo,
            'affiliateFee', 
                SUBSTRING(memo FROM '^(?:ADD|[+]|a):(?:[^:]*:){3}(\d{1,5}?)(?::|$)')::int,
            'affiliateAddress',
                SUBSTRING(memo FROM '^(?:ADD|[+]|a):(?:[^:]*:){2}([^:]+)')
            ) AS meta
    FROM stake_events
    UNION ALL
    -- Pending `add`s will be removed when not pending anymore
    SELECT
        event_id,
        block_timestamp,
        'addLiquidity' AS action_type,
        'PL:' || rune_addr || ':' || pool :: text AS main_ref,
        non_null_array(rune_addr, asset_addr) AS addresses,
        non_null_array(rune_tx, asset_tx) AS transactions,
        ARRAY[pool, 'THOR.RUNE'] :: text[] AS assets,
        ARRAY[pool] :: text[] AS pools,
        transaction_list(
            mktransaction(rune_tx, rune_addr, ('THOR.RUNE', rune_e8)),
            mktransaction(asset_tx, asset_addr, (pool, asset_e8))
            ) AS ins,
        jsonb_build_array() AS outs,
        jsonb_build_array() AS fees,
        jsonb_build_object('status', 'pending') AS meta
    FROM pending_liquidity_events
    WHERE pending_type = 'add'
    ;

CREATE VIEW btcq_indexer_agg.send_actions AS
    SELECT
        event_id,
        block_timestamp,
        'send' AS action_type,
        tx_id :: text AS main_ref,
        ARRAY[from_addr, to_addr] :: text[] AS addresses,
        non_null_array(tx_id) AS transactions,
        ARRAY[asset] :: text[] AS assets,
        NULL :: text[] AS pools,
        jsonb_build_array(mktransaction(tx_id, from_addr, (asset, amount_e8))) AS ins,
        jsonb_build_array(mktransaction(tx_id, to_addr, (asset, amount_e8))) AS outs,
        jsonb_build_array(jsonb_build_object('asset', asset, 'amount', 20000000)) AS fees,
        jsonb_build_object('memo', memo, 'code', code, 'log', raw_log) AS meta
    FROM send_messages;

CREATE VIEW btcq_indexer_agg.thorname_actions AS
    SELECT
        event_id,
        block_timestamp,
        'thorname' AS action_type,
        tx_id :: text AS main_ref,
        non_null_array(address, owner, sender) AS addresses,
        non_null_array(tx_id) AS transactions,
        ARRAY['THOR.RUNE'] :: text[] AS assets,
        NULL :: text[] AS pools,
        jsonb_build_array(mktransaction(tx_id, sender, ('THOR.RUNE', (fund_amount_e8 + registration_fee_e8) :: bigint))) AS ins,
        jsonb_build_array() AS outs,
        jsonb_build_array(jsonb_build_object('asset', 'THOR.RUNE', 'amount', 20000000)) AS fees,
        jsonb_build_object(
            'thorName', name,
            'address', address,
            'owner', owner,
            'chain', chain,
            'fundAmount', fund_amount_e8,
            'registrationFee', registration_fee_e8,
            'expire', expire,
            'memo', memo,
            'txType', 'thorname'
            ) AS meta
    FROM thorname_change_events;


CREATE VIEW btcq_indexer_agg.trade_actions AS
    SELECT
        event_id,
        block_timestamp,
        'trade' AS action_type,
        tx_id :: text AS main_ref,
        non_null_array(rune_address, asset_address) AS addresses,
        non_null_array(tx_id) AS transactions,
        ARRAY[asset] :: text[] AS assets,
        NULL :: text[] AS pools,
        jsonb_build_array(mktransaction(tx_id, asset_address, (REPLACE(asset, '~', '.'), amount_e8 :: bigint))) AS ins,
        jsonb_build_array() AS outs,
        jsonb_build_array() AS fees,
        NULL :: jsonb AS meta
    FROM trade_account_deposit_events
    UNION ALL
    SELECT
        event_id,
        block_timestamp,
        'trade' AS action_type,
        tx_id :: text AS main_ref,
        non_null_array(rune_address, asset_address) AS addresses,
        non_null_array(tx_id) AS transactions,
        ARRAY[asset] :: text[] AS assets,
        NULL :: text[] AS pools,
        transaction_list(mktransaction(tx_id, rune_address, (asset, amount_e8 :: bigint))) AS ins,
        jsonb_build_array() AS outs,
        jsonb_build_array() AS fees,
        NULL :: jsonb AS meta
    FROM trade_account_withdraw_events;

CREATE VIEW btcq_indexer_agg.secure_actions AS
    SELECT
        event_id,
        block_timestamp,
        'secure' AS action_type,
        tx_id :: text AS main_ref,
        non_null_array(rune_address, asset_address) AS addresses,
        non_null_array(tx_id) AS transactions,
        ARRAY[asset] :: text[] AS assets,
        NULL :: text[] AS pools,
        jsonb_build_array(mktransaction(tx_id, asset_address, (REPLACE(asset, '-', '.'), amount_e8 :: bigint))) AS ins,
        jsonb_build_array() AS outs,
        jsonb_build_array() AS fees,
        NULL :: jsonb AS meta
    FROM secure_asset_deposit_events
    UNION ALL
    SELECT
        event_id,
        block_timestamp,
        'secure' AS action_type,
        tx_id :: text AS main_ref,
        non_null_array(rune_address, asset_address) AS addresses,
        non_null_array(tx_id) AS transactions,
        ARRAY[asset] :: text[] AS assets,
        NULL :: text[] AS pools,
        transaction_list(mktransaction(tx_id, rune_address, (asset, amount_e8 :: bigint))) AS ins,
        jsonb_build_array() AS outs,
        jsonb_build_array() AS fees,
        NULL :: jsonb AS meta
    FROM secure_asset_withdraw_events;

CREATE VIEW btcq_indexer_agg.rune_pool_actions AS
    SELECT
        event_id,
        block_timestamp,
        'runePoolDeposit' AS action_type,
        tx_id :: text AS main_ref,
        non_null_array(rune_addr) AS addresses,
        non_null_array(tx_id) AS transactions,
        ARRAY['THOR.RUNE'] :: text[] AS assets,
        NULL :: text[] AS pools,
        jsonb_build_array(mktransaction(tx_id, rune_addr, ('THOR.RUNE', amount_e8 :: bigint))) AS ins,
        jsonb_build_array() AS outs,
        jsonb_build_array(jsonb_build_object('asset', 'THOR.RUNE', 'amount', 20000000)) AS fees,
        jsonb_build_object(
            'units', units
            ) AS meta
    FROM rune_pool_deposit_events
    UNION ALL
    SELECT
        event_id,
        block_timestamp,
        'runePoolWithdraw' AS action_type,
        tx_id :: text AS main_ref,
        non_null_array(rune_addr) AS addresses,
        non_null_array(tx_id) AS transactions,
        ARRAY['THOR.RUNE'] :: text[] AS assets,
        NULL :: text[] AS pools,
        jsonb_build_array() AS ins,
        jsonb_build_array(mktransaction(tx_id, rune_addr, ('THOR.RUNE', amount_e8 :: bigint))) AS outs,
        jsonb_build_array(jsonb_build_object('asset', 'THOR.RUNE', 'amount', 20000000)) AS fees,
        jsonb_build_object(
            'units', units,
            'basisPoints', basis_points,
            'affiliateBasisPts', affiliate_basis_pts,
            'affiliateAmount', affiliate_amount_e8,
            'affiliateAddr', affiliate_addr     
            ) AS meta
    FROM rune_pool_withdraw_events;

CREATE VIEW btcq_indexer_agg.bond_actions AS
    SELECT
        event_id,
        block_timestamp,
        CASE
            WHEN SUBSTRING(LOWER(memo) FROM '^(.*?):')::text = ANY('{bond}') THEN 'bond'
            WHEN SUBSTRING(LOWER(memo) FROM '^(.*?):')::text = ANY('{unbond}') THEN 'unbond' 
            ELSE 'unknown'
        END AS action_type,
        tx :: text AS main_ref,
        non_null_array(from_addr, to_addr, SUBSTRING(LOWER(memo) FROM '^(?:bond|unbond):(?:[^:]*:){0}([^:]+)')) AS addresses,
        non_null_array(tx) AS transactions,
        ARRAY[asset] :: text[] AS assets,
        NULL :: text[] AS pools,
        jsonb_build_array(mktransaction(tx, from_addr, (asset, asset_e8 :: bigint))) AS ins,
        jsonb_build_array(mktransaction(tx, to_addr, (asset, e8 :: bigint))) AS outs,
        jsonb_build_array(jsonb_build_object('asset', 'THOR.RUNE', 'amount', 20000000)) AS fees,
        jsonb_build_object(
            'memo', memo,
            'nodeAddress', SUBSTRING(LOWER(memo) FROM '^(?:bond|unbond):(?:[^:]*:){0}([^:]+)'),
            'provider', SUBSTRING(LOWER(memo) FROM '^(?:bond):(?:[^:]*:){1}([^:]+)'),
            'fee', SUBSTRING(LOWER(memo) FROM '^(?:bond):(?:[^:]*:){2}([^:]+)')
            ) AS meta
    FROM bond_events
    WHERE _tx_type != 'unknown';

CREATE VIEW btcq_indexer_agg.failed_actions AS
    SELECT
        event_id,
        block_timestamp,
        'failed' action_type,
        tx_id :: text AS main_ref,
        non_null_array(from_addr) AS addresses,
        non_null_array(tx_id) AS transactions,
        ARRAY[asset] :: text[] AS assets,
        NULL :: text[] AS pools,
        jsonb_build_array(mktransaction(tx_id, from_addr, (asset, amount_e8 :: bigint))) AS ins,
        jsonb_build_array() AS outs,
        jsonb_build_array(jsonb_build_object('asset', 'THOR.RUNE', 'amount', 20000000)) AS fees,
        jsonb_build_object(
            'memo', memo,
            'reason', reason,
            'code', code
            ) AS meta
    FROM failed_deposit_messages;

CREATE VIEW btcq_indexer_agg.contract_actions AS
    SELECT
        event_id,
        block_timestamp,
        'contract' action_type,
        tx_id :: text AS main_ref,
        non_null_array(sender, contract_address) AS addresses,
        non_null_array(tx_id) AS transactions,
        ARRAY['THOR.RUNE']  :: text[] AS assets,
        NULL :: text[] AS pools,
        jsonb_build_array(mktransaction(tx_id, sender, ('THOR.RUNE', 0 :: bigint))) AS ins,
        jsonb_build_array(mktransaction(tx_id, contract_address, ('THOR.RUNE', 0 :: bigint))) AS outs,
        jsonb_build_array() AS fees,
        jsonb_build_object(
            'type', contract_type,
            'attributes', attributes,
            'msg', msg,
            'funds', funds
        ) AS meta
    FROM wasm_contracts_events;

CREATE VIEW btcq_indexer_agg.instantiate_actions AS
    SELECT
        event_id,
        block_timestamp,
        'contract' action_type,
        tx_id :: text AS main_ref,
        non_null_array(sender, contract_address, admin_address) AS addresses,
        non_null_array(tx_id) AS transactions,
        ARRAY['THOR.RUNE']  :: text[] AS assets,
        NULL :: text[] AS pools,
        jsonb_build_array(mktransaction(tx_id, sender, ('THOR.RUNE', 0 :: bigint))) AS ins,
        jsonb_build_array(mktransaction(tx_id, contract_address, ('THOR.RUNE', 0 :: bigint))) AS outs,
        jsonb_build_array() AS fees,
        jsonb_build_object(
            'type', label,
            'attributes', msg,
            'admin_address', admin_address,
            'funds', funds
            ) AS meta
    FROM instantiate_events;

CREATE VIEW btcq_indexer_agg.tcy_actions AS
    SELECT
        event_id,
        block_timestamp,
        'tcy_claim' AS action_type,
        tx_id :: text AS main_ref,
        non_null_array(l1_address, rune_address) AS addresses,
        non_null_array(tx_id) AS transactions,
        ARRAY[asset, 'THOR.TCY'] :: text[] AS assets,
        NULL :: text[] AS pools,
        jsonb_build_array(mktransaction(tx_id, l1_address, (asset, 0 :: bigint))) AS ins,
        jsonb_build_array(mktransaction(tx_id, rune_address, ('THOR.TCY', tcy_amt :: bigint))) AS outs,
        jsonb_build_array() AS fees,
        jsonb_build_object(
            'memo', memo
            ) AS meta
    FROM tcy_claim_events
    UNION ALL
    SELECT
        event_id,
        block_timestamp,
        'tcy_stake' AS action_type,
        tx_id :: text AS main_ref,
        non_null_array(rune_address) AS addresses,
        non_null_array(tx_id) AS transactions,
        ARRAY['THOR.TCY'] :: text[] AS assets,
        NULL :: text[] AS pools,
        jsonb_build_array(mktransaction(tx_id, rune_address, ('THOR.TCY', amount :: bigint))) AS ins,
        jsonb_build_array() AS outs,
        jsonb_build_array() AS fees,
        jsonb_build_object(
            'memo', memo
            ) AS meta
    FROM tcy_stake_events
    UNION ALL
    SELECT
        event_id,
        block_timestamp,
        'tcy_unstake' AS action_type,
        tx_id :: text AS main_ref,
        non_null_array(rune_address) AS addresses,
        non_null_array(tx_id) AS transactions,
        ARRAY['THOR.TCY'] :: text[] AS assets,
        NULL :: text[] AS pools,
        jsonb_build_array() AS ins,
        jsonb_build_array(mktransaction(tx_id, rune_address, ('THOR.TCY', amount :: bigint))) AS outs,
        jsonb_build_array() AS fees,
        jsonb_build_object(
            'memo', memo
            ) AS meta
    FROM tcy_unstake_events;

CREATE VIEW btcq_indexer_agg.limit_swap_actions AS
    SELECT
        event_id,
        block_timestamp,
        'limit_swap' AS action_type,
        tx_id :: text AS main_ref,
        non_null_array(from_addr, to_addr) AS addresses,
        non_null_array(tx_id) AS transactions,
        ARRAY[from_asset, to_asset] :: text[] AS assets,
        NULL :: text[] AS pools,
        jsonb_build_array(mktransaction(tx_id, from_addr, (from_asset, from_e8 :: bigint))) AS ins,
        CASE 
            WHEN to_e8 > 0 THEN jsonb_build_array(mktransaction(tx_id, to_addr, (to_asset, to_e8 :: bigint)))
            ELSE jsonb_build_array()
        END AS outs,
        jsonb_build_array() AS fees,
        jsonb_build_object(
            'memo', memo
            ) AS meta
    FROM limit_swap_events;

CREATE VIEW btcq_indexer_agg.rebond_actions AS
    SELECT
        event_id,
        block_timestamp,
        'rebond' AS action_type,
        tx_id :: text AS main_ref,
        non_null_array(old_bond_address, new_bond_address, node_address) AS addresses,
        non_null_array(tx_id) AS transactions,
        ARRAY['THOR.RUNE'] :: text[] AS assets,
        NULL :: text[] AS pools,
        jsonb_build_array(mktransaction(tx_id, old_bond_address, ('THOR.RUNE', amount :: bigint))) AS ins,
        jsonb_build_array(mktransaction(tx_id, to_addr, ('THOR.RUNE', amount :: bigint))) AS outs,
        jsonb_build_array() AS fees,
        jsonb_build_object(
            'newBondAddress', new_bond_address,
            'nodeAddress', node_address,
            'memo', memo
            ) AS meta
    FROM rebond_events;

--
-- Procedures for updating actions
--

CREATE PROCEDURE btcq_indexer_agg.insert_actions(t1 bigint, t2 bigint)
LANGUAGE plpgsql AS $BODY$
BEGIN

    EXECUTE $$ INSERT INTO btcq_indexer_agg.actions
    SELECT * FROM btcq_indexer_agg.switch_actions
        WHERE $1 <= block_timestamp AND block_timestamp < $2 ON CONFLICT DO NOTHING $$ USING t1, t2;

    EXECUTE $$ INSERT INTO btcq_indexer_agg.actions
    SELECT * FROM btcq_indexer_agg.refund_actions
        WHERE $1 <= block_timestamp AND block_timestamp < $2 ON CONFLICT DO NOTHING $$ USING t1, t2;

    EXECUTE $$ INSERT INTO btcq_indexer_agg.actions
    SELECT * FROM btcq_indexer_agg.donate_actions
        WHERE $1 <= block_timestamp AND block_timestamp < $2 ON CONFLICT DO NOTHING $$ USING t1, t2;

    EXECUTE $$ INSERT INTO btcq_indexer_agg.actions
    SELECT * FROM btcq_indexer_agg.withdraw_actions
        WHERE $1 <= block_timestamp AND block_timestamp < $2 ON CONFLICT DO NOTHING $$ USING t1, t2;

    EXECUTE $$ INSERT INTO btcq_indexer_agg.actions
    SELECT * FROM btcq_indexer_agg.swap_actions
        WHERE $1 <= block_timestamp AND block_timestamp < $2 ON CONFLICT DO NOTHING $$ USING t1, t2;

    EXECUTE $$ INSERT INTO btcq_indexer_agg.actions
    SELECT * FROM btcq_indexer_agg.addliquidity_actions
        WHERE $1 <= block_timestamp AND block_timestamp < $2 ON CONFLICT DO NOTHING $$ USING t1, t2;
    
    EXECUTE $$ INSERT INTO btcq_indexer_agg.actions
    SELECT * FROM btcq_indexer_agg.send_actions
        WHERE $1 <= block_timestamp AND block_timestamp < $2 ON CONFLICT DO NOTHING $$ USING t1, t2;

    EXECUTE $$ INSERT INTO btcq_indexer_agg.actions
    SELECT * FROM btcq_indexer_agg.thorname_actions
        WHERE $1 <= block_timestamp AND block_timestamp < $2 ON CONFLICT DO NOTHING $$ USING t1, t2;

    EXECUTE $$ INSERT INTO btcq_indexer_agg.actions
    SELECT * FROM btcq_indexer_agg.trade_actions
        WHERE $1 <= block_timestamp AND block_timestamp < $2 ON CONFLICT DO NOTHING $$ USING t1, t2;

    EXECUTE $$ INSERT INTO btcq_indexer_agg.actions
    SELECT * FROM btcq_indexer_agg.rune_pool_actions
        WHERE $1 <= block_timestamp AND block_timestamp < $2 ON CONFLICT DO NOTHING $$ USING t1, t2;

    EXECUTE $$ INSERT INTO btcq_indexer_agg.actions
    SELECT * FROM btcq_indexer_agg.bond_actions
        WHERE $1 <= block_timestamp AND block_timestamp < $2 ON CONFLICT DO NOTHING $$ USING t1, t2;
    
    EXECUTE $$ INSERT INTO btcq_indexer_agg.actions
    SELECT * FROM btcq_indexer_agg.failed_actions
        WHERE $1 <= block_timestamp AND block_timestamp < $2 ON CONFLICT DO NOTHING $$ USING t1, t2;
    
    EXECUTE $$ INSERT INTO btcq_indexer_agg.actions
    SELECT * FROM btcq_indexer_agg.secure_actions
        WHERE $1 <= block_timestamp AND block_timestamp < $2 ON CONFLICT DO NOTHING $$ USING t1, t2;
    
    EXECUTE $$ INSERT INTO btcq_indexer_agg.actions
    SELECT * FROM btcq_indexer_agg.contract_actions
        WHERE $1 <= block_timestamp AND block_timestamp < $2 ON CONFLICT DO NOTHING $$ USING t1, t2;
    
    EXECUTE $$ INSERT INTO btcq_indexer_agg.actions
    SELECT * FROM btcq_indexer_agg.instantiate_actions
        WHERE $1 <= block_timestamp AND block_timestamp < $2 ON CONFLICT DO NOTHING $$ USING t1, t2;

    EXECUTE $$ INSERT INTO btcq_indexer_agg.actions
    SELECT * FROM btcq_indexer_agg.tcy_actions
        WHERE $1 <= block_timestamp AND block_timestamp < $2 ON CONFLICT DO NOTHING $$ USING t1, t2;
    
    EXECUTE $$ INSERT INTO btcq_indexer_agg.actions
    SELECT * FROM btcq_indexer_agg.limit_swap_actions
        WHERE $1 <= block_timestamp AND block_timestamp < $2 ON CONFLICT DO NOTHING $$ USING t1, t2;

    EXECUTE $$ INSERT INTO btcq_indexer_agg.actions
    SELECT * FROM btcq_indexer_agg.rebond_actions
        WHERE $1 <= block_timestamp AND block_timestamp < $2 ON CONFLICT DO NOTHING $$ USING t1, t2;
END
$BODY$;

-- TODO(muninn): Check the pending logic regarding nil rune address
CREATE PROCEDURE btcq_indexer_agg.trim_pending_actions(t1 bigint, t2 bigint)
LANGUAGE plpgsql AS $BODY$
BEGIN
    DELETE FROM btcq_indexer_agg.actions AS a
    USING stake_events AS s
    WHERE
        t1 <= s.block_timestamp AND s.block_timestamp < t2
        AND a.event_id <= s.event_id
        AND a.main_ref = 'PL:' || s.rune_addr || ':' || s.pool;

    DELETE FROM btcq_indexer_agg.actions AS a
    USING pending_liquidity_events AS pw
    WHERE
        t1 <= pw.block_timestamp AND pw.block_timestamp < t2
        AND a.event_id <= pw.event_id
        AND pw.pending_type = 'withdraw'
        AND a.main_ref = 'PL:' || pw.rune_addr || ':' || pw.pool;
END
$BODY$;

-- TODO(huginn): Remove duplicates from these lists?
CREATE PROCEDURE btcq_indexer_agg.actions_add_outbounds(t1 bigint, t2 bigint)
LANGUAGE plpgsql AS $BODY$
BEGIN
    UPDATE outbound_events as o
    SET
        internal = TRUE
    FROM (
        SELECT *
        FROM swap_events
        WHERE t1 <= block_timestamp AND block_timestamp < t2
        ) as a
    WHERE
        o.in_tx = a.tx AND
        o.block_timestamp = a.block_timestamp AND
        a.from_asset = 'THOR.RUNE' AND
        o.asset_e8 = a.from_e8 AND
        o.asset = 'THOR.RUNE'
    ;

    UPDATE btcq_indexer_agg.actions AS a
    SET
        addresses = a.addresses || o.froms || o.tos,
        transactions = a.transactions || array_remove(o.transactions, NULL),
        assets = a.assets || o.assets,
        outs = a.outs || o.outs
    FROM (
        SELECT
            in_tx,
            array_agg(from_addr :: text) AS froms,
            array_agg(to_addr :: text) AS tos,
            array_agg(tx :: text) AS transactions,
            array_agg(asset :: text) AS assets,
            jsonb_agg(btcq_indexer_agg.out_tx(tx, to_addr, TRUNC(event_id / 1e10)::text, internal, (asset, asset_e8))) AS outs
        FROM outbound_events
        WHERE t1 <= block_timestamp AND block_timestamp < t2 AND internal IS NOT TRUE
        GROUP BY in_tx
        ) AS o
    WHERE
        o.in_tx = a.main_ref AND a.action_type != 'switch' AND a.action_type != 'limit_swap';
END
$BODY$;

CREATE PROCEDURE btcq_indexer_agg.actions_add_trade_deposit(t1 bigint, t2 bigint)
LANGUAGE plpgsql AS $BODY$
BEGIN
    UPDATE btcq_indexer_agg.actions AS a
    SET
        addresses = (with b as (select unnest(a.addresses || o.addresses) b) select array_agg(distinct b) from b),
        assets = (with b as (select unnest(a.assets || o.assets) b) select array_agg(distinct b) from b), 
        outs = CASE
            WHEN o.outs #>> '{0, "coins", 0, "asset"}' = a.outs #>> '{0, "coins", 0, "asset"}' 
            THEN jsonb_set(a.outs, '{0, "coins", 0, "amount"}',
                to_jsonb((a.outs #> '{0, "coins", 0, "amount"}')::bigint + 
                (o.outs #> '{0, "coins", 0, "amount"}')::bigint))
            ELSE
                a.outs || o.outs
            END
    FROM (
        SELECT
            tx_id,
            rune_address AS addresses,
            asset AS assets,
            btcq_indexer_agg.out_tx(tx_id, rune_address, NULL, TRUE, (asset, amount_e8)) AS outs
        FROM (
            SELECT DISTINCT ON (tx_id, rune_address, asset) 
                SUM(amount_e8)::bigint as amount_e8, 
                tx_id, 
                rune_address,
                asset 
            FROM trade_account_deposit_events 
            WHERE t1 <= block_timestamp AND block_timestamp < t2
            GROUP BY tx_id, rune_address, asset
            ) AS t
        ) AS o
    WHERE
        o.tx_id = a.main_ref;
END
$BODY$;

CREATE PROCEDURE btcq_indexer_agg.actions_add_secure_deposit(t1 bigint, t2 bigint)
LANGUAGE plpgsql AS $BODY$
BEGIN
    UPDATE btcq_indexer_agg.actions AS a
    SET
        addresses = (with b as (select unnest(a.addresses || o.addresses) b) select array_agg(distinct b) from b),
        assets = (with b as (select unnest(a.assets || o.assets) b) select array_agg(distinct b) from b), 
        outs = CASE
            WHEN o.outs #>> '{0, "coins", 0, "asset"}' = a.outs #>> '{0, "coins", 0, "asset"}' 
            THEN jsonb_set(a.outs, '{0, "coins", 0, "amount"}',
                to_jsonb((a.outs #> '{0, "coins", 0, "amount"}')::bigint + 
                (o.outs #> '{0, "coins", 0, "amount"}')::bigint))
            ELSE
                a.outs || o.outs
            END
    FROM (
        SELECT
            tx_id,
            rune_address AS addresses,
            asset AS assets,
            btcq_indexer_agg.out_tx(tx_id, rune_address, NULL, TRUE, (asset, amount_e8)) AS outs
        FROM (
            SELECT DISTINCT ON (tx_id, rune_address, asset) 
                SUM(amount_e8)::bigint as amount_e8, 
                tx_id, 
                rune_address,
                asset 
            FROM secure_asset_deposit_events 
            WHERE t1 <= block_timestamp AND block_timestamp < t2
            GROUP BY tx_id, rune_address, asset
            ) AS t
        ) AS o
    WHERE
        o.tx_id = a.main_ref;
END
$BODY$;

-- Add streaming details to swap action delete tx_id and event_id from event
CREATE PROCEDURE btcq_indexer_agg.streaming_details(t1 bigint, t2 bigint)
LANGUAGE plpgsql AS $BODY$
BEGIN
    UPDATE btcq_indexer_agg.actions AS a
    SET
        streaming_meta = a.streaming_meta || out
    FROM (
        SELECT DISTINCT ON (tx_id)
            tx_id,
            jsonb_build_object(
                'interval', interval,
                'quantity', quantity,
                'count', count,
                'last_height', last_height,
                'deposit_asset', deposit_asset,
                'deposit_e8', deposit_e8,
                'in_asset', in_asset,
                'in_e8', in_e8,
                'out_asset', out_asset,
                'out_e8', out_e8,
                'failed_swaps', failed_swaps,
                'failed_swap_reasons', failed_swap_reasons
            ) as out
        FROM streaming_swap_details_events
        WHERE t1 <= block_timestamp AND block_timestamp < t2
        ) AS s
    WHERE
        s.tx_id = a.main_ref;
END
$BODY$;

CREATE PROCEDURE btcq_indexer_agg.actions_add_fees(t1 bigint, t2 bigint)
LANGUAGE plpgsql AS $BODY$
BEGIN
    UPDATE btcq_indexer_agg.actions AS a
    SET
        fees = a.fees || f.fees
    FROM (
        SELECT
            tx,
            jsonb_agg(jsonb_build_object('asset', asset, 'amount', asset_e8)) AS fees
        FROM fee_events
        WHERE t1 <= block_timestamp AND block_timestamp < t2
        GROUP BY tx
        ) AS f
    WHERE
        f.tx = a.main_ref;
END
$BODY$;

CREATE FUNCTION btcq_indexer_agg.add_streaming_logs() RETURNS trigger
LANGUAGE plpgsql AS $BODY$
DECLARE
    streaming_swap btcq_indexer_agg.actions;
BEGIN
    -- Look up the current state of the streaming_swap
    SELECT * FROM btcq_indexer_agg.actions
    WHERE main_ref = NEW.main_ref AND action_type = 'swap'
    FOR UPDATE INTO streaming_swap;

    -- Add new streaming swap details to old one
    IF streaming_swap.main_ref IS NOT NULL THEN

        IF NEW.ins #> '{0, "coins", 0, "asset"}' = streaming_swap.ins #> '{0, "coins", 0, "asset"}' THEN
            -- should be update specific values on the jsonb
            streaming_swap.ins := jsonb_set(streaming_swap.ins, '{0, "coins", 0, "amount"}',
                to_jsonb((streaming_swap.ins #> '{0, "coins", 0, "amount"}')::bigint + (NEW.ins #> '{0, "coins", 0, "amount"}')::bigint));
            streaming_swap.streaming_meta := jsonb_set(streaming_swap.streaming_meta, '{count}',
                to_jsonb((NEW.streaming_meta #> '{count}')::bigint)); 
        END IF;

        -- TODO: add swap slip
        UPDATE btcq_indexer_agg.actions SET
            ins = streaming_swap.ins,
            meta = streaming_swap.meta || jsonb_build_object('SwapStreaming', true) ||
                jsonb_build_object('liquidityFee', (meta->>'liquidityFee')::bigint + (NEW.meta->>'liquidityFee')::bigint),
            streaming_meta = streaming_swap.streaming_meta
        WHERE main_ref = streaming_swap.main_ref AND action_type = 'swap';
    END IF;


    -- Never fails, just enriches the row to be inserted and updates the `members` table.
    RETURN NEW;
END;
$BODY$;

CREATE TRIGGER add_log_trigger
    BEFORE INSERT ON btcq_indexer_agg.actions
    FOR EACH ROW
    WHEN (NEW.action_type = 'swap')
    EXECUTE FUNCTION btcq_indexer_agg.add_streaming_logs();


CREATE PROCEDURE btcq_indexer_agg.update_actions_interval(t1 bigint, t2 bigint)
LANGUAGE plpgsql AS $BODY$
BEGIN
    CALL btcq_indexer_agg.insert_actions(t1, t2);
    CALL btcq_indexer_agg.trim_pending_actions(t1, t2);
    CALL btcq_indexer_agg.actions_add_outbounds(t1, t2);
    CALL btcq_indexer_agg.actions_add_trade_deposit(t1, t2);
    CALL btcq_indexer_agg.actions_add_secure_deposit(t1, t2);
    CALL btcq_indexer_agg.streaming_details(t1,t2);
    CALL btcq_indexer_agg.actions_add_fees(t1, t2);
END
$BODY$;

INSERT INTO btcq_indexer_agg.watermarks (materialized_table, watermark)
    VALUES ('actions', 0);

CREATE PROCEDURE btcq_indexer_agg.update_actions(w_new bigint)
LANGUAGE plpgsql AS $BODY$
DECLARE
    w_old bigint;
BEGIN
    SELECT watermark FROM btcq_indexer_agg.watermarks WHERE materialized_table = 'actions'
        FOR UPDATE INTO w_old;
    IF w_new <= w_old THEN
        RAISE WARNING 'Updating actions into past: % -> %', w_old, w_new;
        RETURN;
    END IF;
    CALL btcq_indexer_agg.update_actions_interval(w_old, w_new);
    UPDATE btcq_indexer_agg.watermarks SET watermark = w_new WHERE materialized_table = 'actions';
END
$BODY$;
