# User Authentication Feature Design Specification

**Issue**: Closes #121

## Solution Approach

This design extends kiro-krew with a web API server component to provide JWT-based authentication services. The architecture introduces a new `server` command alongside the existing CLI functionality, enabling users to run kiro-krew as both a CLI tool and an authentication service.

### High-Level Architecture

```
kiro-krew CLI (existing)     kiro-krew server (new)
     │                              │
     ├── watch                      ├── HTTP API Server
     ├── status                     ├── JWT Auth Service  
     ├── plan                       ├── Database Layer
     └── ...                        └── Auth Middleware
```

The authentication system will be self-contained within the `internal/auth` package and expose RESTful endpoints through a new server mode.

## Relevant Files

### New Files to Create
- `internal/auth/service.go` - Core JWT authentication logic
- `internal/auth/handlers.go` - HTTP handlers for auth endpoints
- `internal/auth/models.go` - User and token data structures
- `internal/auth/jwt.go` - JWT token creation and validation
- `internal/auth/database.go` - Database operations for users/tokens
- `internal/middleware/auth.go` - JWT validation middleware
- `internal/server/server.go` - HTTP server setup and routing
- `cmd/kiro-krew/cmd/server.go` - Server subcommand
- `migrations/001_create_users.sql` - Database schema
- `migrations/002_create_refresh_tokens.sql` - Refresh tokens schema

### Files to Modify
- `cmd/kiro-krew/cmd/root.go` - Add server subcommand
- `internal/config/config.go` - Add server and database configuration
- `go.mod` - Add required dependencies (jwt, bcrypt, database driver)

### Configuration Files
- `.kiro-krew/config.yaml` - Extend with server settings

## Team Orchestration

The authentication system operates independently of the existing agent orchestration:

1. **Separation of Concerns**: Auth service runs as separate server mode
2. **Database Independence**: Uses local SQLite by default, configurable for other DBs
3. **No Impact on Existing Workflows**: CLI commands remain unchanged
4. **Optional Feature**: Server mode is opt-in via `kiro-krew server` command

## Step-by-Step Task Breakdown

### Task 1: Database Schema and Models
**Acceptance Criteria:**
- Users table with id, email, password_hash, created_at, updated_at
- Refresh_tokens table with id, user_id, token, expires_at, created_at
- Database migration system for schema management
- User and RefreshToken Go structs with proper tags

### Task 2: Core Authentication Service
**Acceptance Criteria:**
- Password hashing using bcrypt with configurable cost
- JWT token generation with RS256 signing
- Token validation and parsing
- User registration with email uniqueness validation
- Login with email/password verification

### Task 3: Database Layer
**Acceptance Criteria:**
- SQLite database connection with configurable path
- User CRUD operations (Create, GetByEmail, GetByID)
- Refresh token management (Create, GetByToken, Delete, CleanExpired)
- Database initialization and migration runner
- Prepared statements for security

### Task 4: HTTP Handlers and Routing
**Acceptance Criteria:**
- POST /auth/register endpoint with input validation
- POST /auth/login endpoint returning access + refresh tokens
- POST /auth/refresh endpoint for token renewal
- POST /auth/logout endpoint for token invalidation
- Proper HTTP status codes and error responses
- Request/response JSON structures

### Task 5: JWT Middleware
**Acceptance Criteria:**
- Authorization header parsing (Bearer token)
- JWT signature validation
- Token expiration checking
- User context injection for downstream handlers
- Protected route demonstration endpoint

### Task 6: Server Infrastructure
**Acceptance Criteria:**
- HTTP server with graceful shutdown
- Route registration and middleware chain
- CORS configuration for development
- Configurable server port and host
- Request logging and error handling

### Task 7: CLI Integration
**Acceptance Criteria:**
- `kiro-krew server` subcommand
- Server configuration in `.kiro-krew/config.yaml`
- Database path and JWT secret configuration
- Help text and usage examples

### Task 8: Configuration and Dependencies
**Acceptance Criteria:**
- Updated go.mod with jwt-go, bcrypt, sqlite driver
- Extended Config struct for server settings
- Database and JWT configuration validation
- Default configuration values

## Validation Commands

```bash
# Start the authentication server
kiro-krew server

# Test user registration
curl -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"password123"}'

# Test user login
curl -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"password123"}'

# Test protected endpoint with JWT token
curl -H "Authorization: Bearer <jwt_token>" \
  http://localhost:8080/protected/profile

# Test token refresh
curl -X POST http://localhost:8080/auth/refresh \
  -H "Content-Type: application/json" \
  -d '{"refresh_token":"<refresh_token>"}'

# Test logout
curl -X POST http://localhost:8080/auth/logout \
  -H "Content-Type: application/json" \
  -d '{"refresh_token":"<refresh_token>"}'
```

## Database Schema

### Users Table
```sql
CREATE TABLE users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    email TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### Refresh Tokens Table
```sql
CREATE TABLE refresh_tokens (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    token TEXT UNIQUE NOT NULL,
    expires_at DATETIME NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
```

## Configuration Extension

```yaml
# .kiro-krew/config.yaml
repo: owner/repo-name
label: kiro-krew
poll_interval: 5m
max_retries: 3

# New server configuration
server:
  enabled: false
  host: "localhost"
  port: 8080
  database_path: ".kiro-krew/auth.db"
  jwt_secret: "your-jwt-secret-key"
  jwt_expiry: "15m"
  refresh_expiry: "7d"
  bcrypt_cost: 12
```

## Security Considerations

1. **Password Storage**: Bcrypt with configurable cost (default 12)
2. **JWT Security**: RS256 signing with proper key management
3. **Token Expiry**: Short access tokens (15m) with refresh mechanism
4. **Input Validation**: Email format, password strength requirements
5. **SQL Injection**: Prepared statements for all database queries
6. **CORS**: Configurable for production deployment
7. **Rate Limiting**: Consider adding for production use

## Implementation Notes

- Use standard library `database/sql` with SQLite driver for simplicity
- JWT implementation using `golang-jwt/jwt/v5` library
- Password hashing with `golang.org/x/crypto/bcrypt`
- HTTP server using standard `net/http` with `gorilla/mux` for routing
- Graceful shutdown handling for the server
- Environment variable overrides for sensitive configuration
