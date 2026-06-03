// MongoDB Initialization Script for Astra Development Environment
// This script runs automatically when the container is first created

// Switch to astra_dev database
db = db.getSiblingDB('astra_dev');

// Create collections with validation (optional)
db.createCollection('users', {
    validator: {
        $jsonSchema: {
            bsonType: 'object',
            required: ['username', 'email'],
            properties: {
                username: {
                    bsonType: 'string',
                    description: 'Username must be a string and is required'
                },
                email: {
                    bsonType: 'string',
                    pattern: '^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}$',
                    description: 'Email must be a valid email address'
                },
                password_hash: {
                    bsonType: 'string',
                    description: 'Password hash must be a string'
                },
                is_active: {
                    bsonType: 'bool',
                    description: 'Active status must be a boolean'
                },
                created_at: {
                    bsonType: 'date',
                    description: 'Created timestamp must be a date'
                },
                updated_at: {
                    bsonType: 'date',
                    description: 'Updated timestamp must be a date'
                }
            }
        }
    }
});

// Create indexes
db.users.createIndex({ email: 1 }, { unique: true });
db.users.createIndex({ username: 1 }, { unique: true });
db.users.createIndex({ created_at: -1 });

// Insert sample data
db.users.insertMany([
    {
        username: 'admin',
        email: 'admin@example.com',
        password_hash: '$2a$10$YourHashedPasswordHere',
        is_active: true,
        created_at: new Date(),
        updated_at: new Date()
    },
    {
        username: 'demo',
        email: 'demo@example.com',
        password_hash: '$2a$10$YourHashedPasswordHere',
        is_active: true,
        created_at: new Date(),
        updated_at: new Date()
    }
]);

print('Astra MongoDB initialization completed successfully');
