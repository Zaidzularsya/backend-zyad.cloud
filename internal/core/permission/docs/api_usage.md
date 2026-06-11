# Panduan Penggunaan API dan Modul Permission

Dokumen ini menjelaskan cara mengonsumsi (consume) endpoint HTTP API permission serta cara menggunakan modul permission checker langsung di level kode program (Go).

---

## 1. HTTP API Endpoints

Semua endpoint API permission didaftarkan di bawah base path `/api/v1`.

### A. List All Permissions
Mengambil daftar seluruh hak akses (permissions) yang terdaftar di dalam sistem.

*   **Endpoint**: `GET /api/v1/permissions`
*   **Headers**: `Content-Type: application/json`
*   **Response (200 OK)**:
    ```json
    {
      "success": true,
      "message": "permissions retrieved",
      "data": [
        {
          "id": "e44d32a1-10fc-4bb8-86d9-768fa5f9175b",
          "name": "permission:read",
          "description": "Read permission data"
        },
        {
          "id": "18f97bc2-3c12-4c22-b5e1-8974a6217431",
          "name": "permission:manage",
          "description": "Manage permission data"
        }
      ]
    }
    ```

### B. Get Role Permissions
Mengambil data detail role beserta daftar hak akses yang terasosiasi dengannya berdasarkan nama role.

*   **Endpoint**: `GET /api/v1/permissions/roles/:role_name`
*   **Headers**: `Content-Type: application/json`
*   **URL Parameter**:
    *   `role_name` (string): Nama role yang ingin dicari (contoh: `superadmin`).
*   **Response (200 OK)**:
    ```json
    {
      "success": true,
      "message": "role permissions retrieved",
      "data": {
        "role_id": "8c12a831-29cf-4a11-89d8-912fc89d231b",
        "role_name": "superadmin",
        "permissions": [
          {
            "id": "e44d32a1-10fc-4bb8-86d9-768fa5f9175b",
            "name": "permission:read",
            "description": "Read permission data"
          },
          {
            "id": "18f97bc2-3c12-4c22-b5e1-8974a6217431",
            "name": "permission:manage",
            "description": "Manage permission data"
          }
        ]
      }
    }
    ```
*   **Response Error (404 Not Found)**:
    ```json
    {
      "success": false,
      "code": "ROLE_NOT_FOUND",
      "message": "role not found"
    }
    ```

### C. Get User Permissions
Mengambil daftar role yang dimiliki user beserta daftar lengkap hak akses gabungan yang dimilikinya berdasarkan ID user.

*   **Endpoint**: `GET /api/v1/permissions/users/:user_id`
*   **Headers**: `Content-Type: application/json`
*   **URL Parameter**:
    *   `user_id` (string/UUID): ID user.
*   **Response (200 OK)**:
    ```json
    {
      "success": true,
      "message": "user permissions retrieved",
      "data": {
        "user_id": "a4d8123c-bb9e-4c12-88ef-23194a2b1cc1",
        "role_names": [
          "superadmin"
        ],
        "permissions": [
          {
            "id": "e44d32a1-10fc-4bb8-86d9-768fa5f9175b",
            "name": "permission:read",
            "description": "Read permission data"
          },
          {
            "id": "18f97bc2-3c12-4c22-b5e1-8974a6217431",
            "name": "permission:manage",
            "description": "Manage permission data"
          }
        ]
      }
    }
    ```

### D. List All Roles
Mengambil daftar seluruh role yang terdaftar di dalam sistem.

*   **Endpoint**: `GET /api/v1/roles`
*   **Headers**: `Content-Type: application/json`
*   **Response (200 OK)**:
    ```json
    {
      "success": true,
      "message": "roles retrieved",
      "data": [
        {
          "id": "8c12a831-29cf-4a11-89d8-912fc89d231b",
          "name": "superadmin",
          "description": "Full platform administrator"
        }
      ]
    }
    ```

### E. Create Permission
Membuat permission baru.

*   **Endpoint**: `POST /api/v1/permissions`
*   **Headers**: `Content-Type: application/json`
*   **Request Body**:
    ```json
    {
      "name": "lead.read",
      "description": "Read access to leads module",
      "module_id": "a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d"
    }
    ```
*   **Response (201 Created)**:
    ```json
    {
      "success": true,
      "message": "permission created",
      "data": {
        "id": "f55d32a1-10fc-4bb8-86d9-768fa5f9175c",
        "name": "lead.read",
        "description": "Read access to leads module",
        "module_id": "a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d"
      }
    }
    ```

### F. Create Role
Membuat role baru.

*   **Endpoint**: `POST /api/v1/roles`
*   **Headers**: `Content-Type: application/json`
*   **Request Body**:
    ```json
    {
      "name": "sales",
      "description": "Sales Executive role"
    }
    ```
*   **Response (201 Created)**:
    ```json
    {
      "success": true,
      "message": "role created",
      "data": {
        "id": "7d12a831-29cf-4a11-89d8-912fc89d231c",
        "name": "sales",
        "description": "Sales Executive role"
      }
    }
    ```

### G. Assign Permissions to Role
Menugaskan/memetakan satu atau beberapa permission ke suatu role.

*   **Endpoint**: `POST /api/v1/roles/:id/permissions`
*   **Headers**: `Content-Type: application/json`
*   **URL Parameter**:
    *   `id` (string/UUID): ID dari Role.
*   **Request Body**:
    ```json
    {
      "permission_ids": [
        "f55d32a1-10fc-4bb8-86d9-768fa5f9175c"
      ]
    }
    ```
*   **Response (200 OK)**:
    ```json
    {
      "success": true,
      "message": "permissions assigned to role"
    }
    ```

### H. Assign Roles to User
Menugaskan/memetakan satu atau beberapa role ke seorang user.

*   **Endpoint**: `POST /api/v1/users/:user_id/roles`
*   **Headers**: `Content-Type: application/json`
*   **URL Parameter**:
    *   `user_id` (string/UUID): ID dari User.
*   **Request Body**:
    ```json
    {
      "role_ids": [
        "7d12a831-29cf-4a11-89d8-912fc89d231c"
      ]
    }
    ```
*   **Response (200 OK)**:
    ```json
    {
      "success": true,
      "message": "roles assigned to user"
    }
    ```

### I. Get Permission Detail
Mengambil data detail dari suatu permission berdasarkan ID-nya.

*   **Endpoint**: `GET /api/v1/permissions/:id`
*   **Headers**: `Content-Type: application/json`
*   **Response (200 OK)**:
    ```json
    {
      "success": true,
      "message": "permission retrieved",
      "data": {
        "id": "f55d32a1-10fc-4bb8-86d9-768fa5f9175c",
        "name": "lead.read",
        "description": "Read access to leads module",
        "module_id": "a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d"
      }
    }
    ```

### J. Update Permission
Memperbarui data deskripsi atau modul dari suatu permission.

*   **Endpoint**: `PUT /api/v1/permissions/:id`
*   **Headers**: `Content-Type: application/json`
*   **Request Body**:
    ```json
    {
      "description": "Updated read access description",
      "module_id": "a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d"
    }
    ```
*   **Response (200 OK)**:
    ```json
    {
      "success": true,
      "message": "permission updated",
      "data": {
        "id": "f55d32a1-10fc-4bb8-86d9-768fa5f9175c",
        "name": "lead.read",
        "description": "Updated read access description",
        "module_id": "a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d"
      }
    }
    ```

### K. Delete Permission
Menghapus permission dari sistem.

*   **Endpoint**: `DELETE /api/v1/permissions/:id`
*   **Headers**: `Content-Type: application/json`
*   **Response (200 OK)**:
    ```json
    {
      "success": true,
      "message": "permission deleted"
    }
    ```

### L. Get Role Detail
Mengambil data detail role beserta daftar lengkap permission-nya berdasarkan ID role.

*   **Endpoint**: `GET /api/v1/roles/:id`
*   **Headers**: `Content-Type: application/json`
*   **Response (200 OK)**:
    ```json
    {
      "success": true,
      "message": "role retrieved",
      "data": {
        "id": "7d12a831-29cf-4a11-89d8-912fc89d231c",
        "name": "sales",
        "description": "Sales Executive role",
        "permissions": [
          {
            "id": "f55d32a1-10fc-4bb8-86d9-768fa5f9175c",
            "name": "lead.read",
            "description": "Read access to leads module"
          }
        ]
      }
    }
    ```

### M. Update Role
Memperbarui deskripsi suatu role.

*   **Endpoint**: `PUT /api/v1/roles/:id`
*   **Headers**: `Content-Type: application/json`
*   **Request Body**:
    ```json
    {
      "description": "Updated Sales Executive description"
    }
    ```
*   **Response (200 OK)**:
    ```json
    {
      "success": true,
      "message": "role updated",
      "data": {
        "id": "7d12a831-29cf-4a11-89d8-912fc89d231c",
        "name": "sales",
        "description": "Updated Sales Executive description"
      }
    }
    ```

### N. Delete Role
Menghapus role dari sistem.

*   **Endpoint**: `DELETE /api/v1/roles/:id`
*   **Headers**: `Content-Type: application/json`
*   **Response (200 OK)**:
    ```json
    {
      "success": true,
      "message": "role deleted"
    }
    ```

### O. Revoke Permissions from Role
Mencabut penugasan satu atau beberapa permission dari suatu role.

*   **Endpoint**: `DELETE /api/v1/roles/:id/permissions`
*   **Headers**: `Content-Type: application/json`
*   **Request Body**:
    ```json
    {
      "permission_ids": [
        "f55d32a1-10fc-4bb8-86d9-768fa5f9175c"
      ]
    }
    ```
*   **Response (200 OK)**:
    ```json
    {
      "success": true,
      "message": "permissions revoked from role"
    }
    ```

### P. Revoke Roles from User
Mencabut penugasan satu atau beberapa role dari seorang user.

*   **Endpoint**: `DELETE /api/v1/users/:user_id/roles`
*   **Headers**: `Content-Type: application/json`
*   **Request Body**:
    ```json
    {
      "role_ids": [
        "7d12a831-29cf-4a11-89d8-912fc89d231c"
      ]
    }
    ```
*   **Response (200 OK)**:
    ```json
    {
      "success": true,
      "message": "roles revoked from user"
    }
    ```

---



## 2. Penggunaan di Level Kode (Go)

Selain diekspos sebagai HTTP API, modul permission juga dapat langsung digunakan di dalam service atau router modul bisnis lainnya untuk melakukan proteksi hak akses.

### A. Mengamankan Endpoint Menggunakan Middleware Gin
Anda dapat memproteksi route Gin menggunakan middleware `Require` yang telah disediakan di `internal/core/permission/middleware`.

```go
package user

import (
	"github.com/gin-gonic/gin"
	permissionmw "zyad.cloud/internal/core/permission/middleware"
)

func RegisterRoutes(router gin.IRoutes, checker permissionmw.PermissionChecker) {
	// Membutuhkan hak akses 'user.read'
	router.GET("/users", 
		permissionmw.Require(checker, "user.read"), 
		handler.ListUsers,
	)

	// Membutuhkan hak akses 'user.create' dan 'user.write' sekaligus
	router.POST("/users", 
		permissionmw.Require(checker, "user.create", "user.write"), 
		handler.CreateUser,
	)
}
```

> [!IMPORTANT]
> Middleware `Require` mengambil ID user saat ini melalui key context `user_id`. Pastikan middleware autentikasi (JWT parser) sudah dijalankan terlebih dahulu dan menyimpan ID user menggunakan `permissionmw.SetUserID(c, userID)`.

### B. Validasi Hak Akses Langsung di Level Service
Jika Anda memerlukan validasi hak akses granular di dalam logic internal service, Anda dapat menyuntikkan `permissionservice.Service` dan memanggil method `Can`.

```go
package service

import (
	"context"
	"errors"
)

type PermissionChecker interface {
	Can(ctx context.Context, userID string, requiredPermissions []string) error
}

type UserService struct {
	permChecker PermissionChecker
}

func (s *UserService) DeleteUser(ctx context.Context, actorID string, targetUserID string) error {
	// Verifikasi apakah actorID memiliki permission 'user.delete'
	if err := s.permChecker.Can(ctx, actorID, []string{"user.delete"}); err != nil {
		return err // Mengembalikan error FORBIDDEN jika hak akses tidak cukup
	}

	// Lanjutkan proses penghapusan...
	return nil
}
```
