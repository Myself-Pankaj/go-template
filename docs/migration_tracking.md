# How Migration Tracking Works

## What Happens Every Time You Start Your Server

```
1. Server Starts
   ↓
2. Connects to Database
   ↓
3. RunMigrations() is called
   ↓
4. Checks "gorp_migrations" table
   ↓
5. Compares with files in ./migrations/
   ↓
6. Only runs NEW migrations
   ↓
7. Updates "gorp_migrations" table
   ↓
8. Server continues starting
```

## The Magic: Migration Tracking Table

When you run migrations for the **first time**, sql-migrate automatically creates a table called `gorp_migrations`:

```sql
-- This table is created automatically
CREATE TABLE gorp_migrations (
    id VARCHAR(255) NOT NULL PRIMARY KEY,
    applied_at TIMESTAMP WITH TIME ZONE
);
```

### Example Data in `gorp_migrations`:

| id                         | applied_at          |
| -------------------------- | ------------------- |
| 001_create_users_table.sql | 2025-10-12 10:30:00 |
| 002_add_users_index.sql    | 2025-10-12 10:30:01 |

## Timeline Example

### First Run (Fresh Database)

```bash
make dev

# Output:
# Running database migrations...
# Applied migrations successfully {"count": 1}  ← Runs 001_create_users_table.sql
# Database migrations completed successfully
```

**Database State:**

- ✅ `users` table created
- ✅ `gorp_migrations` table created
- ✅ Record added: `001_create_users_table.sql`

---

### Second Run (Same Database)

```bash
make dev

# Output:
# Running database migrations...
# Applied migrations successfully {"count": 0}  ← No new migrations!
# Database migrations completed successfully
```

**What Happened:**

1. Checked `./migrations/` folder → found `001_create_users_table.sql`
2. Checked `gorp_migrations` table → already has `001_create_users_table.sql`
3. Skipped it (already applied)
4. Count = 0 (no new migrations to apply)

---

### Third Run (After Adding New Migration)

**You create:** `migrations/002_add_users_index.sql`

```bash
make dev

# Output:
# Running database migrations...
# Applied migrations successfully {"count": 1}  ← Runs only the new one!
# Database migrations completed successfully
```

**What Happened:**

1. Found 2 files in `./migrations/`
2. Checked `gorp_migrations` → only has `001_create_users_table.sql`
3. Runs **only** `002_add_users_index.sql` (the new one)
4. Updates `gorp_migrations` table

## Check Migration Status

You can check which migrations have been applied:

```sql
-- Connect to your database and run:
SELECT * FROM gorp_migrations ORDER BY applied_at;
```

Output:

```
id                           | applied_at
-----------------------------|---------------------------
001_create_users_table.sql   | 2025-10-12 10:30:00+00
002_add_users_index.sql      | 2025-10-12 14:15:23+00
```

## Key Benefits

### ✅ Safe to Run Multiple Times

```bash
make dev  # Run 1: Creates tables
make dev  # Run 2: Skips (already done)
make dev  # Run 3: Skips (already done)
```

**No errors, no duplicates, no problems!**

### ✅ Team Collaboration

- Developer A creates migration `003_add_email_index.sql`
- Developer A commits and pushes
- Developer B pulls the code
- Developer B runs `make dev`
- Only migration `003` runs on Developer B's database

### ✅ Production Deployments

```bash
# Deploy new version with migrations
git pull
make prod  # Automatically applies new migrations only
```

## Why This Approach is Better

### ❌ Without Migration Tracking:

```bash
# First run
make dev  # Creates users table ✅

# Second run
make dev  # ERROR: table "users" already exists ❌
```

### ✅ With Migration Tracking (Current Setup):

```bash
# First run
make dev  # Creates users table ✅

# Second run
make dev  # Skips, already applied ✅

# Add new migration
make dev  # Runs only the new one ✅
```

## Migration File Naming Convention

**Important:** Files are run in **alphabetical order**:

```
migrations/
├── 001_create_users_table.sql      ← Runs first
├── 002_add_users_index.sql         ← Runs second
├── 003_create_posts_table.sql      ← Runs third
└── 004_add_foreign_keys.sql        ← Runs fourth
```

Use timestamps for guaranteed order:

```
migrations/
├── 20251012_100000_create_users_table.sql
├── 20251012_140000_add_users_index.sql
└── 20251013_090000_create_posts_table.sql
```

## Common Scenarios

### Scenario 1: Clean Database

```bash
# Drop and recreate database
psql -U postgres -c "DROP DATABASE myapp_dev;"
psql -U postgres -c "CREATE DATABASE myapp_dev;"

# Start server
make dev
# Result: All migrations run (count = number of files)
```

### Scenario 2: Up-to-Date Database

```bash
# No new migrations added
make dev
# Result: No migrations run (count = 0)
```

### Scenario 3: New Migration Added

```bash
# Added: migrations/005_new_feature.sql
make dev
# Result: Only new migration runs (count = 1)
```

## Verify Your Setup

Run this in your database:

```sql
-- Check if tracking table exists
SELECT EXISTS (
    SELECT FROM information_schema.tables
    WHERE table_name = 'gorp_migrations'
);
-- Should return: true (after first migration run)

-- See all applied migrations
SELECT * FROM gorp_migrations;

-- Count applied migrations
SELECT COUNT(*) FROM gorp_migrations;
```

## Summary

**Question:** Why don't migrations run again?

**Answer:** They DO run every time, but:

- ✅ sql-migrate **checks** `gorp_migrations` table
- ✅ **Compares** with files in `./migrations/`
- ✅ Only **applies NEW** migrations
- ✅ **Updates** tracking table
- ✅ **Safe** to run unlimited times

This is called **idempotent migrations** - safe to run multiple times with the same result!
