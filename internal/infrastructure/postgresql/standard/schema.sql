-- ============================================================================
-- STANDARD RELATIONAL SCHEMA (CLASSICAL POSTGRESQL)
-- ============================================================================
-- Relational database table direct access.
-- Stores each domain field in a dedicated SQL column.
-- ============================================================================

CREATE TABLE IF NOT EXISTS users_standard (
    id UUID PRIMARY KEY,
    first_name VARCHAR(255) NOT NULL,
    last_name VARCHAR(255) NOT NULL,
    birth_date BIGINT NOT NULL,
    active BOOLEAN NOT NULL,
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL,
    deleted_at BIGINT NOT NULL
);

-- Disable autovacuum to prevent background I/O interference during benchmarks
ALTER TABLE users_standard SET (autovacuum_enabled = false);
