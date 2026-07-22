-- ============================================================================
-- TSID RELATIONAL POSTGRESQL SCHEMA (64-BIT BIGINT TSID)
-- ============================================================================
-- Relational database table direct access with 64-bit TSID primary key.
-- Stores each domain field in a dedicated SQL column.
-- Keys: Stored as 64-bit integer TSIDs inside PostgreSQL's BIGINT column type.
-- ============================================================================

CREATE TABLE IF NOT EXISTS users_tsid (
    id BIGINT PRIMARY KEY, -- 64-bit TSID (Time-Sortable ID) integer
    first_name VARCHAR(255) NOT NULL,
    last_name VARCHAR(255) NOT NULL,
    birth_date BIGINT NOT NULL,
    active BOOLEAN NOT NULL,
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL,
    deleted_at BIGINT NOT NULL
);
