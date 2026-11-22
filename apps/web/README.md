# apps/web

财报只读页：表格排序/筛选、行操作、CSV、i18n（中/英）、深浅色，并对齐新契约（`packages/api-client` → `/v1/meta` / `/v1/facts` / `/v1/platform/status`）。页面顶部展示 Sheets/PG **存储并存**快照。

```bash
# 终端 1：API
AUTH_MODE=bypass STORAGE_MODE=dual \
  DATABASE_URL=postgres://earnings:earnings@127.0.0.1:5432/earnings?sslmode=disable \
  go run ./cmd/server -conf trpc_go.yaml

# 终端 2：静态页（必须从仓库根起服务，以便加载 ../../packages/api-client）
make web
# 打开 http://127.0.0.1:5173/apps/web/
```

契约：[docs/api-contract.md](../../docs/api-contract.md)。并存验收：[docs/coexistence-acceptance.md](../../docs/coexistence-acceptance.md)。
