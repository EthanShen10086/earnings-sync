# Monorepo 布局

```
earnings-sync/
├── cmd/                  # Go 可执行入口（server / orchestrate / doctor …）
├── internal/             # Go 业务与适配器（单一 go.mod）
├── apps/
│   ├── api/              # API 边界说明 + 转调 Makefile（不另立 module）
│   ├── web/              # 静态只读页（ESM → packages/api-client）
│   ├── mobile-ios/       # SwiftUI + xcodegen；共享 packages/swift-api-client
│   └── mobile-android/   # Kotlin Compose 骨架
├── packages/
│   ├── api-client/       # JS HTTP 客户端
│   ├── api-types/        # OpenAPI lite
│   └── swift-api-client/ # Swift Package（swift test）
├── migrations/ deploy/ docs/ configs/ testdata/
└── Makefile
```

## 原则

1. **Go 不拆 module**：入口仍在 `cmd/`，HTTP 面在 `internal/httpapi`；`apps/api` 只做边界文档与捷径。
2. **前端与原生对齐同一契约**：`docs/api-contract.md` + `packages/api-types/openapi-lite.json`。
3. **大重构目录可以，但 `make test`（`go test ./...`）必须绿**；布局守卫见 `internal/monorepo`。
4. **Sheets 旧链路保留**，PG/API/多端展示并存——验收步骤见 [coexistence-acceptance.md](./coexistence-acceptance.md)。

## 常用命令

```bash
make test                 # Go 单测（含 monorepo 布局）
make monorepo-check       # 目录 / 契约文件存在性
make web                  # 静态站（根路径，含 apps/web + packages）
make mobile-swift-test    # Swift 包单测
make mobile-ios           # xcodegen + iOS Simulator build（需 Xcode）
make mobile-android       # assembleDebug（需 JDK 17 + Android SDK）
```
