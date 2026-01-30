INSERT INTO midgard_agg.watermarks (materialized_table, watermark)
    VALUES ('rune_pool', 0);

CREATE TABLE midgard_agg.rune_pool_log (
    member_id text NOT NULL,
    change_type text NOT NULL,
    basis_points bigint,
    units_delta bigint NOT NULL,
    units_total bigint NOT NULL,
    -- rune fields
    rune_addr text,
    rune_e8_delta bigint,
    rune_tx text,
    --
    event_id bigint NOT NULL,
    block_timestamp bigint NOT NULL
);

-- Intended to be inserted into `rune_pool_log` with the totals and other missing info filled out
-- by the trigger.
CREATE VIEW midgard_agg.rune_pool_log_partial AS (
    SELECT * FROM (
        SELECT
            rune_addr AS member_id,
            'add' AS change_type,
            NULL::bigint AS basis_points,
            units AS units_delta,
            NULL::bigint AS units_total,
            rune_addr,
            amount_e8 AS rune_e8_delta,
            tx_id AS rune_tx,
            event_id,
            block_timestamp
        FROM rune_pool_deposit_events
        UNION ALL
        SELECT
            rune_addr AS member_id,
            'withdraw' AS change_type,
            basis_points AS basis_points,
            -units AS units_delta,
            NULL::bigint AS units_total,
            rune_addr AS rune_addr,
            -amount_e8 AS rune_e8_delta,
            tx_id AS rune_tx,
            event_id,
            block_timestamp
        FROM rune_pool_withdraw_events
    ) AS x
    ORDER BY block_timestamp, change_type
);

CREATE TABLE midgard_agg.rune_pool_members (
    member_id text NOT NULL,
    units_total bigint NOT NULL,
    -- rune fields
    rune_addr text,
    rune_e8_deposit bigint NOT NULL,
    added_rune_e8_total bigint NOT NULL,
    withdrawn_rune_e8_total bigint NOT NULL,
    --
    first_added_timestamp bigint,
    last_added_timestamp bigint,
    PRIMARY KEY (member_id)
)
WITH (fillfactor = 90);

CREATE INDEX ON midgard_agg.rune_pool_members (rune_addr);

CREATE TABLE midgard_agg.rune_pool_members_count (
    count bigint NOT NULL,
    block_timestamp bigint NOT NULL,
    PRIMARY KEY (block_timestamp)
);

CREATE INDEX ON midgard_agg.rune_pool_members_count (block_timestamp DESC);

CREATE FUNCTION midgard_agg.add_rune_pool_members_log() RETURNS trigger
LANGUAGE plpgsql AS $BODY$
DECLARE
    member midgard_agg.rune_pool_members%ROWTYPE;
BEGIN
    -- Look up the current state of the rune pool member
    SELECT * FROM midgard_agg.rune_pool_members
        WHERE member_id = NEW.member_id
        FOR UPDATE INTO member;

    -- If this is a new rune pool member, fill out its fields
    IF member.member_id IS NULL THEN
        member.member_id = NEW.member_id;
        member.rune_addr = NEW.rune_addr;
        member.units_total = 0;
        member.added_rune_e8_total = 0;
        member.withdrawn_rune_e8_total = 0;
        member.rune_e8_deposit = 0;

        -- Add to members count table
        INSERT INTO midgard_agg.rune_pool_members_count VALUES
        (
            COALESCE(
                (
                    SELECT count + 1 FROM midgard_agg.rune_pool_members_count
                    ORDER BY block_timestamp DESC LIMIT 1
                ),
                1
            ),
            NEW.block_timestamp 
        ) ON CONFLICT (block_timestamp) DO UPDATE SET count = EXCLUDED.count;
    END IF;

    member.units_total := member.units_total + COALESCE(NEW.units_delta, 0);
    NEW.units_total := member.units_total;

    IF NEW.change_type = 'add' THEN
        member.added_rune_e8_total := member.added_rune_e8_total + NEW.rune_e8_delta;

        -- Calculate deposited Value here
        member.rune_e8_deposit := member.rune_e8_deposit + NEW.rune_e8_delta;

        member.first_added_timestamp := COALESCE(member.first_added_timestamp, NEW.block_timestamp);
        member.last_added_timestamp := NEW.block_timestamp;
    END IF;

    IF NEW.change_type = 'withdraw' THEN
        -- Deltas are negative here
        member.withdrawn_rune_e8_total := member.withdrawn_rune_e8_total - NEW.rune_e8_delta;
        -- Calculate deposited Value here
        member.rune_e8_deposit := ((10000 - NEW.basis_points)/10000) * member.rune_e8_deposit;
    END IF;

    -- Update the `rune_pool_members` table:
    IF member.units_total = 0 THEN
        DELETE FROM midgard_agg.rune_pool_members
        WHERE member_id = member.member_id;

        -- Remove member from rune_pool_members count table
        INSERT INTO midgard_agg.rune_pool_members_count VALUES
        (
            (
                SELECT count - 1 FROM midgard_agg.rune_pool_members_count
                ORDER BY block_timestamp DESC LIMIT 1
            ),
            NEW.block_timestamp
        )
        ON CONFLICT (block_timestamp) DO UPDATE SET count = EXCLUDED.count;
    ELSE
        INSERT INTO midgard_agg.rune_pool_members VALUES (member.*)
        ON CONFLICT (member_id) DO UPDATE SET
            -- Note, `EXCLUDED` is exactly the `rune_pool_member` variable here
            units_total = EXCLUDED.units_total,
            rune_addr = EXCLUDED.rune_addr,
            rune_e8_deposit = EXCLUDED.rune_e8_deposit,
            added_rune_e8_total = EXCLUDED.added_rune_e8_total,
            withdrawn_rune_e8_total = EXCLUDED.withdrawn_rune_e8_total,
            first_added_timestamp = EXCLUDED.first_added_timestamp,
            last_added_timestamp = EXCLUDED.last_added_timestamp;
    END IF;

    -- Never fails, just enriches the row to be inserted and updates the `rune_pool_member` table.
    RETURN NEW;
END;
$BODY$;

CREATE TRIGGER add_log_trigger
    BEFORE INSERT ON midgard_agg.rune_pool_log
    FOR EACH ROW
    EXECUTE FUNCTION midgard_agg.add_rune_pool_members_log();


CREATE PROCEDURE midgard_agg.update_rune_pool_members_interval(t1 bigint, t2 bigint)
LANGUAGE plpgsql AS $BODY$
BEGIN
    INSERT INTO midgard_agg.rune_pool_log (
        SELECT * FROM midgard_agg.rune_pool_log_partial
        WHERE t1 <= block_timestamp AND block_timestamp < t2
        ORDER BY event_id
    );
END
$BODY$;

CREATE PROCEDURE midgard_agg.update_rune_pool_members(w_new bigint)
LANGUAGE plpgsql AS $BODY$
DECLARE
    w_old bigint;
BEGIN
    SELECT watermark FROM midgard_agg.watermarks WHERE materialized_table = 'rune_pool'
        FOR UPDATE INTO w_old;
    IF w_new <= w_old THEN
        RAISE WARNING 'Updating rune pool members into past: % -> %', w_old, w_new;
        RETURN;
    END IF;
    CALL midgard_agg.update_rune_pool_members_interval(w_old, w_new);
    UPDATE midgard_agg.watermarks SET watermark = w_new WHERE materialized_table = 'rune_pool';
END
$BODY$;
