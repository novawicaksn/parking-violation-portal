# Parking Violation Portal

Parking Violation Portal adalah aplikasi web untuk simulasi pengelolaan pelanggaran parkir. Project ini dibuat sebagai bagian dari Tan Digital Assignment menggunakan arsitektur frontend dan backend yang terpisah.

## Tech Stack

### Backend
- Go 1.22+
- PostgreSQL
- REST API

### Frontend
- React
- TypeScript
- Vite

### Database
- PostgreSQL 14+

---

# Features

- Officer membuat data pelanggaran parkir
- Perhitungan denda berdasarkan rule aktif
- Publish versi rule baru
- Member membayar denda (Mock Payment)
- Riwayat transaksi pelanggaran
- Rule Version Snapshot
- Dashboard Officer & Member

---

# Project Structure

```
parking-violation-portal
│
├── backend
│   ├── cmd
│   ├── internal
│   ├── go.mod
│   └── ...
│
├── frontend
│   ├── src
│   ├── public
│   ├── package.json
│   └── ...
│
├── design
│
└── README.md
```

---

# Prerequisites

Install terlebih dahulu:

- Go 1.22+
- Node.js 20+
- Docker Desktop (opsional)
- PostgreSQL 14+

---

# Menjalankan Database

## Menggunakan Docker

```bash
docker run --name parking-postgres ^
-e POSTGRES_PASSWORD=postgres ^
-e POSTGRES_DB=parking_violation ^
-p 5432:5432 ^
-d postgres:16
```

Cek apakah database sudah berjalan:

```bash
docker ps
```

Harus muncul container:

```
parking-postgres
```

---

# Menjalankan Backend

Masuk ke folder backend.

```bash
cd backend
```

Install dependency:

```bash
go mod tidy
```

Jalankan:

```bash
go run cmd/api/main.go
```

Jika berhasil akan muncul:

```
api gateway listening on :8080
```

---

## Jika Port 8080 Sudah Dipakai

Cek:

```powershell
netstat -ano | findstr :8080
```

Jika port digunakan aplikasi lain (misalnya Docker), jalankan backend di port lain.

Windows PowerShell:

```powershell
$env:API_ADDR=":8081"
go run cmd/api/main.go
```

Jika berhasil:

```
api gateway listening on :8081
```

---

# Menjalankan Frontend

Masuk ke folder frontend.

```bash
cd frontend
```

Install package:

```bash
npm install
```

Jika backend berjalan di port **8080**:

```bash
npm run dev
```

Frontend akan berjalan di

```
http://localhost:5173
```

---

## Jika Backend Menggunakan Port 8081

Sebelum menjalankan frontend:

Windows PowerShell

```powershell
$env:VITE_API_URL="http://localhost:8081"
npm run dev
```

atau buat file

```
frontend/.env
```

isi:

```
VITE_API_URL=http://localhost:8081
```

---

## Jika Port 5173 Sudah Dipakai

Misalnya digunakan Docker.

Cek:

```powershell
netstat -ano | findstr :5173
```

atau

```powershell
docker ps
```

Jika ada container menggunakan 5173:

```
guyub-frontend
```

Stop:

```powershell
docker stop guyub-frontend
```

Atau jalankan frontend pada port lain:

```bash
npm run dev -- --port 5175
```

---

# Default URL

Backend

```
http://localhost:8080
```

atau

```
http://localhost:8081
```

Frontend

```
http://localhost:5173
```

atau

```
http://localhost:5175
```

---

# API Test

Membuka daftar user:

```
http://localhost:8080/api/users
```

atau

```
http://localhost:8081/api/users
```

Jika berhasil akan tampil JSON seperti:

```json
[
  {
    "name": "Member One"
  },
  {
    "name": "Officer One"
  }
]
```

---

# Demo

1. Login sebagai **Officer One**
2. Publish Rule
3. Tambah Pelanggaran
4. Ganti ke **Member One**
5. Pilih metode pembayaran
6. Bayar pelanggaran
7. Lihat perubahan saldo
8. Lihat riwayat transaksi

---

# Notes

- Authentication masih menggunakan mock (`X-User-ID`).
- Payment menggunakan simulasi (`success` dan `failed`).
- Foto pelanggaran disimpan dalam format Base64.
- Rule yang sudah digunakan pada pelanggaran tidak berubah meskipun terdapat rule baru.

---

# Design

Diagram tersedia pada folder:

```
design/
```

- flow.drawio
- flow.svg
- erd.drawio
- erd.svg

---

# Author

Nova Putri Wicaksono