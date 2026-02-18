-- version 42

CREATE EXTENSION IF NOT EXISTS timescaledb CASCADE;

----------
-- Clean up

DROP SCHEMA IF EXISTS btcq_indexer_agg CASCADE;
DROP SCHEMA IF EXISTS btcq_indexer CASCADE;

----------
-- Fresh start

CREATE SCHEMA btcq_indexer;

-- Check that the newly created schema is the one we are going to work with.
-- If someone uses a non-standard set up, like using a different postgres user name, it's better
-- to abort at this point and let them know that it's not going to work.
DO $$ BEGIN
    ASSERT (SELECT current_schema()) = 'btcq_indexer', 'current_schema() is not btcq_indexer';
END $$;


CREATE TABLE constants (
  key TEXT NOT NULL,
  value BYTEA NOT NULL,
  PRIMARY KEY (key)
);

CREATE TABLE block_log (
    height          BIGINT NOT NULL,
    timestamp       BIGINT NOT NULL,
    hash            BYTEA NOT NULL,
    agg_state       BYTEA,
    PRIMARY KEY (height),
    UNIQUE (timestamp)
);

CREATE TABLE blocks (
    height              BIGINT NOT NULL PRIMARY KEY,
    finalized_events    JSONB NOT NULL,
    txs                 JSONB NOT NULL,
    CONSTRAINT blocks_height_fkey FOREIGN KEY (height) REFERENCES block_log (height) ON DELETE CASCADE
);

-- For hypertables with an integer 'time' dimension (as opposed to TIMESTAMPTZ),
-- TimescaleDB requires an 'integer_now' function to be set to use continuous aggregates.
-- We use the following function, 'current_nano', as the 'integer_now' function
-- for all of our hypertables.
--
-- This function is only comes into play if one uses TimescaleDB's automatic refresh policies
-- for continuous aggregates. As we trigger refreshes directly from Midgard, what this
-- function does is basically irrelevant, so we choose to return the most directly
-- corresponding notion of 'now'.
--
-- An alternative approach would be to get the latest block timestamp from 'block_log' or some
-- other table and use TimescaleDB's automatic refresh policies. (The downside is that it gets
-- harder to control, if for example we want to suspend refreshing, etc.)
CREATE FUNCTION current_nano() RETURNS BIGINT
LANGUAGE SQL STABLE AS $$
    SELECT CAST(1000000000 * EXTRACT(EPOCH FROM CURRENT_TIMESTAMP) AS BIGINT)
$$;

CREATE PROCEDURE setup_hypertable(t regclass)
LANGUAGE SQL
AS $$
    SELECT create_hypertable(t, 'block_timestamp',
        chunk_time_interval => (40 * 24 * 60 * 60 * 1000000000 :: BIGINT));
    SELECT set_integer_now_func(t, 'current_nano');
$$;

----------
-- Types and functions

-- The standard PostgreSQL 'date_trunc(field, timestamp)' function,
--  but takes and returns 'nanos from epoch'
CREATE FUNCTION nano_trunc(field TEXT, ts BIGINT) RETURNS BIGINT
LANGUAGE SQL IMMUTABLE AS $$
    SELECT CAST(1000000000 * EXTRACT(EPOCH FROM date_trunc(field, to_timestamp(ts / 1000000000))) AS BIGINT)
$$;

-- Various time/nano/height related helper functions.
-- We don't rely on these, but they are very useful during development and debugging.
CREATE FUNCTION ts_nano(t timestamptz) RETURNS bigint
LANGUAGE SQL IMMUTABLE AS $$
    SELECT CAST(1000000000 * EXTRACT(EPOCH FROM t) AS bigint)
$$;

CREATE FUNCTION nano_ts(t bigint) RETURNS timestamptz
LANGUAGE SQL IMMUTABLE AS $$
    SELECT to_timestamp(t/1e9);
$$;

CREATE FUNCTION height_nano(h bigint) RETURNS bigint
LANGUAGE SQL STABLE AS $$
    SELECT timestamp FROM btcq_indexer.block_log WHERE height = h;
$$;

CREATE FUNCTION last_height() RETURNS bigint
LANGUAGE SQL STABLE AS $$
    SELECT height FROM block_log ORDER BY height DESC LIMIT 1;
$$;

-- Highest possible `event_id` with `block_timestamp` <= `t`.
CREATE FUNCTION nano_event_id_up(t bigint) RETURNS bigint
LANGUAGE SQL STABLE AS $$
    SELECT (height + 1) * 1e10 - 1
    FROM btcq_indexer.block_log
    WHERE timestamp <= t
    ORDER BY timestamp DESC
    LIMIT 1;
$$;

-- Lowest possible `event_id` with `block_timestamp` >= `t`.
CREATE FUNCTION nano_event_id_down(t bigint) RETURNS bigint
LANGUAGE SQL STABLE AS $$
    SELECT height * 1e10
    FROM btcq_indexer.block_log
    WHERE t <= timestamp
    ORDER BY timestamp ASC
    LIMIT 1;
$$;

-- For use in `actions` aggregation.

CREATE TYPE coin_rec AS (asset text, amount bigint);

CREATE FUNCTION non_null_array(VARIADIC elems text[])
RETURNS text[] LANGUAGE SQL IMMUTABLE AS $$
    SELECT array_remove(elems, NULL)
$$;

CREATE FUNCTION coins(VARIADIC coins coin_rec[])
RETURNS jsonb[] LANGUAGE SQL IMMUTABLE AS $$
    SELECT array_agg(jsonb_build_object('asset', asset, 'amount', amount))
    FROM unnest(coins)
    WHERE amount > 0
$$;

CREATE FUNCTION mktransaction(
    txid text,
    address text,
    VARIADIC coins coin_rec[]
) RETURNS jsonb LANGUAGE SQL IMMUTABLE AS $$
    SELECT jsonb_build_object(
        'txID', txid,
        'address', address,
        'coins', coins(VARIADIC coins)
        )
$$;

-- TODO(huginn): better condition in WHERE
CREATE FUNCTION transaction_list(VARIADIC txs jsonb[])
RETURNS jsonb LANGUAGE SQL IMMUTABLE AS $$
    SELECT COALESCE(jsonb_agg(tx), '[]' :: jsonb)
    FROM unnest(txs) AS t(tx)
    WHERE tx->>'coins' <> 'null';
$$;

----------
-- Main hypertables

-- Sparse table for depths.
-- Only those height/pool pairs are filled where there is a change.
-- For missing values, use the latest existing height for a pool.
-- Asset and QBTC are filled together, it's not needed to look back for them separately.
CREATE TABLE block_pool_depths (
    pool                TEXT NOT NULL,
    asset_e8            BIGINT NOT NULL,
    qbtc_e8             BIGINT NOT NULL,
    synth_e8            BIGINT NOT NULL,
    block_timestamp     BIGINT NOT NULL
);

CALL setup_hypertable('block_pool_depths');
CREATE INDEX ON block_pool_depths (pool, block_timestamp DESC);


CREATE TABLE rewards_events (
    bond_e8         BIGINT NOT NULL,
    validator       TEXT,
    event_id        BIGINT NOT NULL,
    block_timestamp BIGINT NOT NULL
);

CALL setup_hypertable('rewards_events');

CREATE TABLE rewards_event_entries (
    pool                TEXT NOT NULL,
    qbtc_e8             BIGINT NOT NULL,
    -- saver_e8 is the total amount earned in the paralel synth pool. 
    -- Rows having saver_e8 field do not come from reward events, 
    -- but from donate events with the memo "THOR-SAVERS-YIELD"
    saver_e8            BIGINT NOT NULL, 
    event_id            BIGINT NOT NULL,
    block_timestamp     BIGINT NOT NULL
);

CALL setup_hypertable('rewards_event_entries');

-- CosmWasm Contract events

CREATE TABLE wasm_contracts_events (
    tx_id                   TEXT,
    contract_address        TEXT,
    contract_type           TEXT,
    sender                  TEXT,
    attributes              jsonb,
    msg                     jsonb,
    funds                   TEXT,
    event_id                BIGINT NOT NULL,
    block_timestamp         BIGINT NOT NULL
);

CREATE INDEX ON wasm_contracts_events (contract_address);
CREATE INDEX ON wasm_contracts_events (contract_type);
CALL setup_hypertable('wasm_contracts_events');

CREATE TABLE instantiate_events (
    tx_id                   TEXT,
    contract_address        TEXT,
    admin_address           TEXT,
    code_id                 BIGINT,
    sender                  TEXT,
    label                   TEXT,
    msg                     jsonb,
    funds                   TEXT,
    event_id                BIGINT NOT NULL,
    block_timestamp         BIGINT NOT NULL
);

CALL setup_hypertable('instantiate_events');

CREATE TABLE transfer_events (
    from_addr           TEXT NOT NULL,
    to_addr             TEXT NOT NULL,
    asset               TEXT DEFAULT 'QBTC.QBTC',
    amount_e8           BIGINT NOT NULL,
    event_id            BIGINT NOT NULL,
    block_timestamp     BIGINT NOT NULL
);

CALL setup_hypertable('transfer_events');

CREATE TABLE qbtc_price (
    qbtc_price_e8       BIGINT NOT NULL,
    block_timestamp     BIGINT NOT NULL
);

CREATE INDEX ON qbtc_price (block_timestamp DESC);

CREATE TABLE pool_events (
    asset               TEXT NOT NULL,
    status              TEXT NOT NULL,
    event_id            BIGINT NOT NULL,
    block_timestamp     BIGINT NOT NULL
);

CALL setup_hypertable('pool_events');

CREATE TABLE set_mimir_events (
    key                 TEXT NOT NULL,
    value               TEXT NOT NULL,
    event_id            BIGINT NOT NULL,
    block_timestamp     BIGINT NOT NULL
);

CALL setup_hypertable('set_mimir_events');
