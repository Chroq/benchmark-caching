-- ============================================================================
-- STANDARD RELATIONAL SCHEMA (CLASSICAL POSTGRESQL)
-- ============================================================================
-- Relational database table direct access.
-- Stores each domain field in a dedicated SQL column.
-- Keys: Stored as 16-byte binary UUID v7 (RFC 9562) inside PostgreSQL's 128-bit UUID column type.
-- ============================================================================

DROP TABLE IF EXISTS users_standard CASCADE;

CREATE TABLE IF NOT EXISTS users_standard (
    id UUID PRIMARY KEY, -- 16-byte binary UUID v7 identifier (RFC 9562)
    first_name VARCHAR(255) NOT NULL,
    last_name VARCHAR(255) NOT NULL,
    birth_date BIGINT NOT NULL,
    active BOOLEAN NOT NULL,
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL,
    deleted_at BIGINT NOT NULL
);

-- Disable autovacuum to prevent background I/O interference during benchmarks
-- ALTER TABLE users_standard SET (autovacuum_enabled = false);


