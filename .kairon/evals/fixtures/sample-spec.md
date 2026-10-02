# User Authentication Feature

## Overview
Add JWT-based authentication system to allow users to login and access protected resources.

## Requirements
1. User registration with email/password
2. JWT token generation on login
3. Protected routes that require authentication
4. Token refresh mechanism
5. Password hashing with bcrypt

## Files to Modify
- `internal/auth/` (new package)
- `internal/api/handlers.go`
- `internal/middleware/auth.go` (new)
- `cmd/server/main.go`

## Database Changes
- Add users table with id, email, password_hash, created_at
- Add refresh_tokens table

## API Endpoints
- POST /auth/register
- POST /auth/login  
- POST /auth/refresh
- POST /auth/logout

## Success Criteria
- Users can register and login
- JWT tokens are properly validated
- Protected endpoints require valid tokens
- Passwords are securely hashed
