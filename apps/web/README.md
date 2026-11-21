# apps/web

财报只读页骨架：表格排序/筛选、行操作、CSV 导出占位、i18n（中/英）、深浅色、多端自适应 CSS。鉴权默认走 API `AUTH_MODE=bypass`，无密钥可演示。

```bash
# 终端 1：API
AUTH_MODE=bypass STORAGE_MODE=dual \
  DATABASE_URL=postgres://earnings:earnings@127.0.0.1:5432/earnings?sslmode=disable \
  go run ./cmd/server -conf trpc_go.yaml

# 终端 2：静态页
make web
# 打开 http://127.0.0.1:5173
```

契约见 [docs/api-contract.md](../../docs/api-contract.md)。
