## NOT Contest Backend by Syrnik Interactive

This service provides API for checking out and purchasing items for [Not Back Contest](https://contest.notco.in/dev-backend)

## Architecture

- Clean architecture with separation into layers (transport, service, storage)
- Minimal dependencies (only Redis and Postgres clients)
- Proprietary HTTP router without frameworks

---

## Project Highlights

- Using Lua scripts in Redis for atomic operations
- Background recording of purchases via worker pool
- Caching in Redis for fast operations
- Graceful shutdown with timeouts

---
## Scalability

- Worker pool for asynchronous purchase recording
- Redis pipelining to reduce network round-trips and increase throughput
- Efficient use of database connections via pooling and short-lived access

---
## Error Handling

Structured error handling using contextual wrapping.  
Implemented via `util.Wrap` (located at `pkg/util/errors.go`), which uses `fmt.Errorf` with `%w` and the `runtime` package to include caller context.

- Function names and parameters automatically included in error messages
- Preserves original error for traceability and introspection (`errors.Is`, `errors.As`)
- Applied consistently across service and storage layers

---
## How to run
```
docker compose up --build
```
or
```
go run cmd/server/main.go
```
The server will be accessible on port `:8080`.


---
## Environment Configuration

```
REDIS_HOST="redis"         # Hostname for Redis server
REDIS_PORT=6379            # Port for Redis connections
REDIS_PASSWORD=""          # Authentication secret (empty if disabled)
REDIS_DB=0                 # Database index (0-15)

POSTGRES_HOST="postgres"      # Database server hostname
POSTGRES_PORT=5432            # Port for PostgreSQL connections
POSTGRES_USER="postgres"      # Database administrator username
POSTGRES_PASSWORD="postgres"  # Sensitive credentials - change in production!
POSTGRES_DB="postgres"        # Default database name
POSTGRES_SSL_MODE="disable"   # Encryption: disable/require/verify-full
POSTGRES_MAX_CONNECTIONS=20   # Connection pool size
```

---

## Performance tests

To run it, you need to run the commands

### FULL Mode (Sequential)
```bash  
go run tests/tests.go -mode=full
```  

Results:
```  
Total Requests: 20001
Successful Checkouts: 10000  
Successful Purchases: 10000  
Failed Requests: 1  
Requests Per Second (RPS): 757.89  
Error Rate: 0.00%  
Average Latency: 1.242586ms  
P95 Latency: 3.0064ms  
P99 Latency: 3.3682ms  
```

### SIM Mode (Concurrent)
```bash  
go run tests/tests.go -mode=sim
```  

Results:
```  
Total Requests: 19550  
Successful Checkouts: 9550  
Successful Purchases: 9549  
Failed Requests: 451  
Requests Per Second (RPS): 3583.29  
Error Rate: 2.31%  
Average Latency: 14.465341ms  
P95 Latency: 28.2867ms  
P99 Latency: 196.6124ms  
```

---
A name that isn’t mentioned — but somehow, everyone knows 🥖
He’s there, in the quiet corners of the code.
The signs are scattered. The trail is real.
Find the eggs. All of them.

---
