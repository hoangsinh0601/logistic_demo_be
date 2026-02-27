# Inventory Management System — Backend

API backend cho hệ thống quản lý kho vận, tài chính, thuế và quy trình phê duyệt.

## Tech Stack

| Layer     | Technology                                     |
| --------- | ---------------------------------------------- |
| Language  | Go 1.25                                        |
| Framework | Gin                                            |
| ORM       | GORM                                           |
| Database  | PostgreSQL 15                                  |
| Auth      | JWT (Access + Refresh Token, HttpOnly Cookies) |
| Docs      | Swagger (swag)                                 |
| Container | Docker + Docker Compose                        |
| Realtime  | WebSocket                                      |

## Kiến trúc

Áp dụng **Clean Architecture** với **Repository Pattern** và **Unit of Work** (context-based transactions):

```
cmd/api/main.go          ← Entry point, DI wiring
internal/
├── model/               ← Domain models (GORM structs)
├── repository/           ← Data access layer (12 repos + TransactionManager)
├── service/              ← Business logic layer (10 services)
├── handler/              ← HTTP handlers / controllers (9 handlers)
├── middleware/            ← Auth (JWT), RBAC, Permission guards
├── database/             ← DB connection
├── config/               ← App config
└── websocket/            ← WebSocket hub
pkg/response/             ← Chuẩn hóa API response
api/swagger/              ← Swagger generated docs
deployments/              ← Dockerfile + docker-compose.yml
configs/                  ← .env file
```

### Repository Pattern

Mọi truy cập DB đều qua interface Repository, không gọi `*gorm.DB` trực tiếp trong Service:

```
Handler → Service → Repository → Database
                  ↘ TransactionManager (Unit of Work)
```

**Transaction propagation** qua `context.Context`: `TransactionManager.RunInTx()` tạo TX, lưu vào ctx. Các repository dùng `GetDB(ctx, rootDB)` để tự động lấy TX nếu có.

## Chức năng chính

### 🏭 Quản lý Kho (Inventory)

- CRUD sản phẩm (`/api/products`)
- Tạo đơn hàng nhập/xuất kho (`/api/orders`)
- Theo dõi tồn kho realtime qua WebSocket
- Row-level locking (`SELECT FOR UPDATE`) khi duyệt đơn

### 💰 Quản lý Chi phí (Expenses)

- Tạo chi phí đa tiền tệ (VND, USD, EUR, JPY...)
- Tự động quy đổi ngoại tệ, tính FCT cho vendor nước ngoài
- Đánh dấu chi phí hợp lệ/không hợp lệ (deductible)

### 🧾 Hóa đơn (Invoices)

- Tự động tạo hóa đơn khi đơn hàng/chi phí được duyệt
- Tính thuế VAT tự động theo Tax Rule đang hiệu lực
- Phụ phí (side fees), mã hóa đơn sequential (`HD2026-XXXX`)

### 📋 Quy trình Phê duyệt (Approvals)

- Workflow phê duyệt 3 loại: `CREATE_ORDER`, `CREATE_PRODUCT`, `CREATE_EXPENSE`
- Duyệt → tự động thực thi hành động (tạo sản phẩm, cập nhật kho, tạo hóa đơn...)
- Từ chối → ghi lý do, không thực thi

### 📊 Thuế (Tax Rules)

- CRUD quy tắc thuế: `VAT_INLAND`, `VAT_INTL`, `FCT`
- Hiệu lực theo thời gian (effective_from / effective_to)
- Kiểm tra trùng lặp (overlapping) khi tạo mới

### 👥 Người dùng & Phân quyền (RBAC)

- Quản lý users (CRUD)
- Roles: `admin`, `manager`, `staff` (có thể mở rộng)
- Permission-based access control (e.g. `inventory.read`, `expenses.write`)
- Middleware: `RequireRole()`, `RequirePermission()`

### 📈 Thống kê & Doanh thu

- Thống kê đơn hàng theo khoảng thời gian
- Top sản phẩm bán chạy
- Báo cáo doanh thu (revenue)

### 📝 Audit Log

- Ghi lại mọi thao tác quan trọng: ai làm gì, lúc nào
- Phân trang, tìm kiếm theo thời gian

## Chạy local

### Yêu cầu

- Go 1.25+
- PostgreSQL 15+ (hoặc Docker)
- Make (optional)

### Bằng Docker (khuyến nghị)

```bash
make docker-up
# Backend: http://localhost:8080
# Swagger: http://localhost:8080/swagger/index.html
```

### Bằng Go trực tiếp

```bash
# 1. Cấu hình .env
cp configs/.env.example configs/.env

# 2. Chạy
make run
# hoặc
go run cmd/api/main.go
```

### Biến môi trường

| Variable       | Default     | Mô tả                                |
| -------------- | ----------- | ------------------------------------ |
| `PORT`         | `8080`      | Port server                          |
| `DB_HOST`      | `localhost` | PostgreSQL host                      |
| `DB_PORT`      | `5432`      | PostgreSQL port                      |
| `DB_USER`      | `postgres`  | PostgreSQL user                      |
| `DB_PASSWORD`  | `postgres`  | PostgreSQL password                  |
| `DB_NAME`      | `postgres`  | Database name                        |
| `DB_SSLMODE`   | `disable`   | SSL mode                             |
| `DATABASE_URL` | —           | Full connection string (ưu tiên hơn) |
| `JWT_SECRET`   | —           | Secret key cho JWT                   |
| `CORS_ORIGINS` | —           | Allowed origins (comma-separated)    |
| `GIN_MODE`     | `debug`     | `debug` / `release`                  |

## API Endpoints

| Method                | Path                         | Mô tả                   |
| --------------------- | ---------------------------- | ----------------------- |
| `POST`                | `/login`                     | Đăng nhập               |
| `POST`                | `/refresh`                   | Refresh token           |
| `POST`                | `/logout`                    | Đăng xuất               |
| `GET`                 | `/me`                        | Thông tin user hiện tại |
| `GET/POST/PUT/DELETE` | `/users/*`                   | CRUD users              |
| `GET/POST`            | `/api/products`              | Sản phẩm                |
| `PUT`                 | `/api/products/:id`          | Cập nhật sản phẩm       |
| `GET/POST`            | `/api/orders`                | Đơn hàng                |
| `GET/POST`            | `/api/expenses`              | Chi phí                 |
| `GET/POST/PUT/DELETE` | `/api/tax-rules/*`           | Quy tắc thuế            |
| `GET/POST`            | `/api/invoices`              | Hóa đơn                 |
| `GET`                 | `/api/approvals`             | Danh sách phê duyệt     |
| `PUT`                 | `/api/approvals/:id/approve` | Duyệt                   |
| `PUT`                 | `/api/approvals/:id/reject`  | Từ chối                 |
| `GET`                 | `/api/roles`                 | Danh sách roles         |
| `GET`                 | `/api/audit-logs`            | Lịch sử thao tác        |
| `GET`                 | `/api/statistics/orders`     | Thống kê đơn hàng       |
| `GET`                 | `/api/invoices/revenue`      | Doanh thu               |
| `GET`                 | `/ws`                        | WebSocket endpoint      |
| `GET`                 | `/health`                    | Health check            |
| `GET`                 | `/swagger/*`                 | API docs                |

> Tất cả endpoint `/api/*` yêu cầu JWT Bearer token, trừ health check và swagger.

## Pagination

Các API dạng list đều hỗ trợ phân trang:

```
GET /api/products?page=1&limit=20
```

Response format:

```json
{
  "status": "success",
  "status_code": 200,
  "data": {
    "products": [...],
    "total": 50,
    "page": 1,
    "limit": 20
  }
}
```

## Make commands

```bash
make build       # Build binary
make run         # Run server
make test        # Run tests
make swagger     # Generate swagger docs
make docker-up   # Start with Docker
make docker-down # Stop Docker
make clean       # Clean build artifacts
```
