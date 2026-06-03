-- MySQL Initialization Script for Astra Development Environment
-- This script runs automatically when the container is first created

USE astra_dev;

-- Create sample schema (optional, remove if not needed)
CREATE TABLE IF NOT EXISTS users (
    id INT AUTO_INCREMENT PRIMARY KEY,
    uuid CHAR(36) DEFAULT (UUID()),
    username VARCHAR(50) UNIQUE NOT NULL,
    email VARCHAR(100) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_users_email (email),
    INDEX idx_users_username (username)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Sample data
INSERT IGNORE INTO users (username, email, password_hash) VALUES
    ('admin', 'admin@example.com', '$2a$10$YourHashedPasswordHere'),
    ('demo', 'demo@example.com', '$2a$10$YourHashedPasswordHere');

-- Log initialization
SELECT 'Astra MySQL initialization completed successfully' AS status;
