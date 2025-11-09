## 🧱 Step 1: Create a Docker network

This lets both containers communicate easily.

```bash
docker network create pg-network
```

---

## 🐘 Step 2: Run PostgreSQL container

Run a new PostgreSQL container and create a new database (say `main_db`):

```bash
docker run -d \
  --name postgres-db \
  --network pg-network \
  -e POSTGRES_USER=testing_user \
  -e POSTGRES_PASSWORD=test_user@2025 \
  -e POSTGRES_DB=main_db \
  -p 5432:5432 \
  postgres:15
```

Explanation:

- `POSTGRES_USER` → your DB username (`*****`)
- `POSTGRES_PASSWORD` → password (`******`)
- `POSTGRES_DB` → name of the initial database (`main_db`)
- Exposes port `5432` so your app or pgAdmin can connect.

---

## 🧭 Step 3: Run pgAdmin container

```bash
docker run -d \
  --name pgadmin \
  --network pg-network \
  -e PGADMIN_DEFAULT_EMAIL=admin@admin.com \
  -e PGADMIN_DEFAULT_PASSWORD=admin \
  -p 5050:80 \
  dpage/pgadmin4
```

Access pgAdmin in your browser:
👉 **[http://localhost:5050](http://localhost:5050)**

Login with:

- **Email:** `admin@admin.com`
- **Password:** `admin`

---

## 🧩 Step 4: Connect pgAdmin to PostgreSQL

In pgAdmin:

1. Right-click **Servers → Register → Server**
2. In the **General** tab:

   - Name: `Postgres DB`

3. In the **Connection** tab:

   - Host name/address: `postgres-db`
   - Port: `5432`
   - Username: `testing_user`
   - Password: `test_user@2025`

Click **Save** ✅

---

## 🧾 Step 5: Verify the database

Once connected, expand:

```
Servers → Postgres DB → Databases → hotel_db
```

You’re ready to go 🎉
