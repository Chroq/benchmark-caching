-- ============================================================================
-- OPTIMIZED CACHE SCHEMA (PRAGMATIC & HIGH-PERFORMANCE)
-- ============================================================================
-- Principles:
-- 1. Simple UNLOGGED table: no WAL writing (near in-memory speed).
-- 2. Unique primary key on `key` (UUID): direct B-Tree lookup (< 1ms).
-- 3. Fillfactor set to 70: reserves page space for updates (HOT Updates).
-- 4. Asynchronous non-blocking cleanup via pg_cron + Batching (DELETE in batches of 10,000).
-- ============================================================================

-- 1. Main cache table creation
CREATE UNLOGGED TABLE IF NOT EXISTS cache_optimized (
    key UUID PRIMARY KEY,
    value BYTEA NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL
) WITH (
    fillfactor = 70,
    autovacuum_enabled = false -- Disabled to avoid undesirable locks during heavy load benchmarks
);

-- 2. Secondary index on expiration timestamp to accelerate background cleanup
CREATE INDEX IF NOT EXISTS idx_cache_optimized_expires_at 
ON cache_optimized (expires_at);


-- ============================================================================
-- SAFETY PURGE FUNCTION (BATCHED DELETE)
-- ============================================================================
-- Why batching? 
-- A massive `DELETE FROM cache_optimized WHERE expires_at < NOW()` across millions 
-- of rows would lock the table. Deleting in batches of 10,000 rows ensures 
-- that reads (`GET`) remain ultra-fast and fluid.
-- ============================================================================

CREATE OR REPLACE FUNCTION purge_expired_cache_keys(batch_size INT DEFAULT 10000)
RETURNS INT AS $$
DECLARE
    deleted_count INT := 0;
    current_deleted INT := 0;
BEGIN
    LOOP
        WITH keys_to_delete AS (
            SELECT key FROM cache_optimized 
            WHERE expires_at <= NOW()
            LIMIT batch_size
            FOR UPDATE SKIP LOCKED -- Does not lock rows currently in use
        )
        DELETE FROM cache_optimized
        WHERE key IN (SELECT key FROM keys_to_delete);
        
        GET DIAGNOSTICS current_deleted = ROW_COUNT;
        deleted_count := deleted_count + current_deleted;

        -- If fewer keys than batch_size were deleted, the purge is complete
        EXIT WHEN current_deleted < batch_size;
    END LOOP;

    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;


-- ============================================================================
-- CRON JOB (PG_CRON) - BACKGROUND CLEANUP
-- ============================================================================
-- Automatically executes the purge function every minute.
-- Only activates if the pg_cron extension is loaded on the DB.
-- ============================================================================

DO $$
BEGIN
    IF to_regclass('cron.job') IS NOT NULL THEN
        -- Unschedule any existing job of the same name (Idempotency)
        PERFORM cron.unschedule(jobid) 
        FROM cron.job 
        WHERE jobname = 'purge_expired_cache_keys_job';

        -- Schedule: Execution every minute (e.g. `* * * * *`)
        PERFORM cron.schedule(
            'purge_expired_cache_keys_job',
            '* * * * *',
            'SELECT purge_expired_cache_keys(10000);'
        );
    END IF;
END
$$;