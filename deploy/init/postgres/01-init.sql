-- PostgreSQL Initialization Script for Astra Development Environment
-- This script runs automatically when the container is first created

-- Create additional databases if needed
-- CREATE DATABASE astra_test;

-- Create extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";  -- For text search
CREATE EXTENSION IF NOT EXISTS "btree_gin"; -- For GIN indexes

-- Create sample schema (optional, remove if not needed)
CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    uuid UUID DEFAULT uuid_generate_v4(),
    username VARCHAR(50) UNIQUE NOT NULL,
    email VARCHAR(100) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_users_username ON users(username);

-- Sample data
INSERT INTO users (username, email, password_hash) VALUES
    ('admin', 'admin@example.com', '$2a$10$YourHashedPasswordHere'),
    ('demo', 'demo@example.com', '$2a$10$YourHashedPasswordHere')
ON CONFLICT (username) DO NOTHING;

-- Grant permissions
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO astra_dev;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO astra_dev;

-- Log initialization
DO $$
BEGIN
    RAISE NOTICE 'Astra PostgreSQL initialization completed successfully';
END $$;
