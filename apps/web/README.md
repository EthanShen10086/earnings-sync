# apps/web

最小财报只读页：请求 API `GET /v1/facts?ticker=AAPL`，展示 PostgreSQL（或 Memory Reader）中的事实行。

```bash
# 终端 1：API（需已 ingest，且 STORAGE_MODE 含 pg 或 Memory 可读）
STORAGE_MODE=dual DATABASE_URL=postgres://earnings:earnings@127.0.0.1:5432/earnings?sslmode=disable \
  go run ./cmd/server -conf trpc_go.yaml

# 终端 2：静态页
cd apps/web && python3 -m http.server 5173 --bind 127.0.0.1
# 打开 http://127.0.0.1:5173
```
