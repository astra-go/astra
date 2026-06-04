-- scripts/init-db.sql — PostgreSQL initialization (used by docker-compose)
-- This runs automatically when the postgres container first starts.

-- Enable extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";

-- Create database (usually already done via POSTGRES_DB, but just in case)
-- Note: Can't create DB inside this script when connecting to an existing one.

-- Set search path
SET search_path TO public;

-- Create blog schema (optional namespace for multi-tenant)
-- CREATE SCHEMA IF NOT EXISTS blog;

-- Grant permissions
GRANT ALL PRIVILEGES ON DATABASE blog TO bloguser;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO bloguser;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO bloguser;

-- Done
