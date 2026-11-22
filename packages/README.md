# packages/

共享包（不另立 Go module；Go 仍在仓库根 `go.mod`）。

| 包 | 用途 |
| --- | --- |
| [`api-client`](./api-client/) | 浏览器 / Node 可用的 HTTP 客户端，对齐 `docs/api-contract.md` |
| [`api-types`](./api-types/) | 契约 JSON Schema / 类型注释，供 web 与移动端对照 |
| [`swift-api-client`](./swift-api-client/) | iOS/macOS Swift Package，`swift test` 可验证 |

`apps/web` 通过相对路径加载 `api-client`；原生壳在 `apps/mobile-ios` / `apps/mobile-android`。
