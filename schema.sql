-- ============================================================================
-- BENCHMARK CACHE DATABASE SCHEMA & AUTO-MAINTENANCE INFRASTRUCTURE
-- ============================================================================
-- Optimized for High-Performance Key-Value Store comparison.
-- Includes:
-- 1. NAIVE SCHEMA: Flat, standard table (logged), VARCHAR(255) key, JSON bytes value.
-- 2. OPTIMIZED SCHEMA: UNLOGGED partitioned parent table, 16-byte UUID key, Protobuf value.
-- 3. Automated partition management via pg_cron.
-- ============================================================================

-- Enable pg_cron extension if not already present
-- Note: 'pg_cron' must also be listed in shared_preload_libraries in postgresql.conf
-- CREATE EXTENSION IF NOT EXISTS pg_cron;

-- ============================================================================
-- PART I: NAIVE CACHE SCHEMA
-- ============================================================================
CREATE TABLE IF NOT EXISTS cache_naive (
    key VARCHAR(255) PRIMARY KEY,
    value BYTEA NOT NULL,
    expires_at TIMESTAMP NOT NULL
);

-- Disable autovacuum to prevent background I/O interference during benchmarks
ALTER TABLE cache_naive SET (autovacuum_enabled = false);

-- Index to query expired items or support pruning
CREATE INDEX IF NOT EXISTS idx_cache_naive_expires_at ON cache_naive(expires_at);

-- ============================================================================
-- PART I.B: STANDARD RELATIONAL SCHEMA
-- ============================================================================
CREATE TABLE IF NOT EXISTS users_standard (
    id UUID PRIMARY KEY,
    first_name VARCHAR(255) NOT NULL,
    last_name VARCHAR(255) NOT NULL,
    birth_date BIGINT NOT NULL,
    active BOOLEAN NOT NULL,
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL,
    deleted_at BIGINT NOT NULL,
    expires_at TIMESTAMP NOT NULL
);

-- Disable autovacuum to prevent background I/O interference during benchmarks
ALTER TABLE users_standard SET (autovacuum_enabled = false);

CREATE INDEX IF NOT EXISTS idx_users_standard_expires_at ON users_standard(expires_at);

-- ============================================================================
-- PART II: OPTIMIZED CACHE SCHEMA
-- ============================================================================
-- Create Parent Table (standard logged table to support unlogged partitions)
-- A composite primary key is used because in partitioned tables, the partition key
-- (expires_at) must be part of any unique/primary key constraints.
CREATE TABLE IF NOT EXISTS cache_optimized_partitioned (
    key UUID NOT NULL,
    value BYTEA NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    PRIMARY KEY (key, expires_at)
) PARTITION BY RANGE (expires_at);

-- ============================================================================
-- PART III: PARTITION MAINTENANCE FUNCTION (manage_cache_partitions)
-- ============================================================================
-- This function performs two main maintenance tasks:
-- 1. Preventive Creation: Provisions partition tables for the next 8 hours.
--    The tables are created as UNLOGGED with a low fillfactor (70) to optimize
--    for high-frequency UPDATEs/INSERTs and HOT (Heap-Only Tuple) updates.
-- 2. Automatic Pruning: Identifies and drops obsolete partition tables whose
--    expiration window has completely passed (end date <= NOW()).
-- ============================================================================
CREATE OR REPLACE FUNCTION manage_cache_partitions()
RETURNS void AS $$
DECLARE
    i integer;
    start_time timestamp;
    end_time timestamp;
    partition_name text;
    r record;
    parts text[];
    part_start timestamp;
    part_end timestamp;
BEGIN
    -- ------------------------------------------------------------------------
    -- PREVENTIVE CREATION (Pre-provision partitions for the next 8 hours)
    -- ------------------------------------------------------------------------
    FOR i IN 0..8 LOOP
        -- Truncate current and future times to the beginning of the hour in UTC
        start_time := date_trunc('hour', timezone('utc', now()) + (i || ' hour')::interval);
        end_time := start_time + interval '1 hour';
        
        -- Construct partition table name: cache_opt_partition_YYYY_MM_DD_HH
        partition_name := 'cache_opt_partition_' || to_char(start_time, 'YYYY_MM_DD_HH24');
        
        -- Create the partition table. 
        -- Specifying UNLOGGED explicitly and setting fillfactor = 70 to reserve page 
        -- space for cache updates/expirations.
        EXECUTE format(
            'CREATE UNLOGGED TABLE IF NOT EXISTS %I PARTITION OF cache_optimized_partitioned ' ||
            'FOR VALUES FROM (%L) TO (%L) WITH (fillfactor = 70, autovacuum_enabled = false);',
            partition_name,
            start_time,
            end_time
        );
    END LOOP;

    -- ------------------------------------------------------------------------
    -- AUTOMATIC PRUNING (Drop partitions older than or equal to NOW() in UTC)
    -- ------------------------------------------------------------------------
    FOR r IN (
        SELECT nmsp_child.nspname AS schema_name, tbl_child.relname AS partition_name
        FROM pg_inherits
        JOIN pg_class tbl_parent ON pg_inherits.inhparent = tbl_parent.oid
        JOIN pg_class tbl_child ON pg_inherits.inhrelid = tbl_child.oid
        JOIN pg_namespace nmsp_child ON tbl_child.relnamespace = nmsp_child.oid
        WHERE tbl_parent.relname = 'cache_optimized_partitioned'
    ) LOOP
        -- Match names conforming to our partition naming convention
        parts := regexp_match(r.partition_name, '^cache_opt_partition_(\d{4})_(\d{2})_(\d{2})_(\d{2})$');
        IF parts IS NOT NULL THEN
            -- Reconstruct the start timestamp from the partition name
            part_start := to_timestamp(parts[1] || '-' || parts[2] || '-' || parts[3] || ' ' || parts[4] || ':00:00', 'YYYY-MM-DD HH24:MI:SS');
            -- End of the 1-hour interval partition
            part_end := part_start + interval '1 hour';
            
            -- If the partition's entire hour is in the past, drop the table to reclaim disk space instantly
            IF part_end <= timezone('utc', now()) THEN
                EXECUTE format('DROP TABLE IF EXISTS %I.%I CASCADE;', r.schema_name, r.partition_name);
            END IF;
        END IF;
    END LOOP;
END;
$$ LANGUAGE plpgsql;

-- Run the partition manager immediately to bootstrap partitions for the first 8 hours
SELECT manage_cache_partitions();

-- ============================================================================
-- PG_CRON SCHEDULE
-- ============================================================================
-- Safe idempotent schedule registration. Only executes if pg_cron is active.
DO $$
BEGIN
    IF to_regclass('cron.job') IS NOT NULL THEN
        -- Unschedule any existing job of the same name
        PERFORM cron.unschedule(jobid) FROM cron.job WHERE jobname = 'manage_cache_partitions_job';

        -- Register the pg_cron job to execute the maintenance task once every hour at minute 0
        PERFORM cron.schedule(
            'manage_cache_partitions_job',
            '0 * * * *',
            'SELECT manage_cache_partitions();'
        );
    END IF;
END
$$;
