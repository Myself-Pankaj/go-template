# User Authentication API Documentation

## Base URL

```
http://localhost:8080/api/v1
```

## Endpoints

### 1. Register User

**POST** `/auth/register`

Register a new user account.

**Request Body:**

```json
{
  "name": "John Doe",
  "email": "john@example.com",
  "phone_number": "1234567890",
  "password": "SecurePassword123"
}
```

**Response (201 Created):**

```json
{
  "success": true,
  "message": "User registered successfully",
  "data": {
    "user": {
      "id": 1,
      "name": "John Doe",
      "email": "john@example.com",
      "phone_number": "1234567890",
      "role": "owner",
      "subscribed": false,
      "plan_type": "free",
      "is_verified": false,
      "trial_start": "2025-10-13T00:00:00Z",
      "trial_end": "2025-11-13T00:00:00Z",
      "created_at": "2025-10-13T10:30:00Z"
    },
    "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
  }
}
```

---

### 2. Login User

**POST** `/auth/login`

Authenticate user and receive JWT token.

**Request Body:**

```json
{
  "email": "john@example.com",
  "password": "SecurePassword123"
}
```

**Response (200 OK):**

```json
{
  "success": true,
  "message": "Login successful",
  "data": {
    "user": {
      "id": 1,
      "name": "John Doe",
      "email": "john@example.com",
      "role": "owner",
      "last_login": "2025-10-13T10:35:00Z"
    },
    "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
  }
}
```

---

### 3. Get User Profile

**GET** `/auth/profile`

Get authenticated user's profile.

**Headers:**

```
Authorization: Bearer <token>
```

**Response (200 OK):**

```json
{
  "success": true,
  "message": "Profile retrieved successfully",
  "data": {
    "id": 1,
    "name": "John Doe",
    "email": "john@example.com",
    "phone_number": "1234567890",
    "role": "owner",
    "subscribed": false,
    "plan_type": "free",
    "is_verified": false,
    "trial_start": "2025-10-13T00:00:00Z",
    "trial_end": "2025-11-13T00:00:00Z",
    "created_at": "2025-10-13T10:30:00Z"
  }
}
```

---

### 4. Update User Profile

**PUT** `/auth/profile`

Update authenticated user's profile information.

**Headers:**

```
Authorization: Bearer <token>
```

**Request Body:**

```json
{
  "name": "John Updated",
  "phone_number": "9876543210"
}
```

**Response (200 OK):**

```json
{
  "success": true,
  "message": "Profile updated successfully",
  "data": {
    "id": 1,
    "name": "John Updated",
    "email": "john@example.com",
    "phone_number": "9876543210",
    "role": "owner"
  }
}
```

---

### 5. Change Password

**POST** `/auth/change-password`

Change authenticated user's password.

**Headers:**

```
Authorization: Bearer <token>
```

**Request Body:**

```json
{
  "old_password": "SecurePassword123",
  "new_password": "NewSecurePassword456"
}
```

**Response (200 OK):**

```json
{
  "success": true,
  "message": "Password changed successfully",
  "data": null
}
```

---

### 6. Logout User

**POST** `/auth/logout`

Logout user (token should be removed client-side).

**Headers:**

```
Authorization: Bearer <token>
```

**Response (200 OK):**

```json
{
  "success": true,
  "message": "Logged out successfully",
  "data": null
}
```

---

### 7. Delete Account

**DELETE** `/auth/account`

Delete authenticated user's account permanently.

**Headers:**

```
Authorization: Bearer <token>
```

**Response (200 OK):**

```json
{
  "success": true,
  "message": "Account deleted successfully",
  "data": null
}
```

---

## Error Responses

### Validation Error (400)

```json
{
  "success": false,
  "message": "Validation failed",
  "errors": [
    {
      "field": "email",
      "message": "email must be a valid email address"
    },
    {
      "field": "password",
      "message": "password must be at least 8 characters long"
    }
  ]
}
```

### Unauthorized (401)

```json
{
  "success": false,
  "message": "Invalid email or password",
  "error": null
}
```

### Conflict (409)

```json
{
  "success": false,
  "message": "Email already registered",
  "error": null
}
```

### Internal Server Error (500)

```json
{
  "success": false,
  "message": "Failed to register user",
  "error": "internal server error"
}
```

---

## Authentication

Most endpoints require JWT authentication. Include the token in the Authorization header:

```
Authorization: Bearer <your-jwt-token>
```

The token is returned after successful registration or login and expires based on your JWT_EXPIRY configuration (default: 24 hours).

---

## Testing with cURL

### Register

```bash
curl -X POST http://localhost:4000/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "name": "John Doe",
    "email": "john@example.com",
    "phone_number": "1234567890",
    "password": "SecurePassword123"
  }'
```

### Login

```bash
curl -X POST http://localhost:4000/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "john@example.com",
    "password": "SecurePassword123"
  }'
```

### Get Profile

```bash
curl -X GET http://localhost:8080/api/v1/auth/profile \
  -H "Authorization: Bearer <your-token>"
```

### Update Profile

```bash
curl -X PUT http://localhost:8080/api/v1/auth/profile \
  -H "Authorization: Bearer <your-token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "John Updated",
    "phone_number": "9876543210"
  }'
```

### Change Password

```bash
curl -X POST http://localhost:8080/api/v1/auth/change-password \
  -H "Authorization: Bearer <your-token>" \
  -H "Content-Type: application/json" \
  -d '{
    "old_password": "SecurePassword123",
    "new_password": "NewSecurePassword456"
  }'
```
