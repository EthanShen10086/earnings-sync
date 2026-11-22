# apps/mobile-ios

原生 iOS（SwiftUI）骨架：共用 `packages/swift-api-client`，展示 `/v1/meta` 存储并存信息与 `/v1/facts`。

## 生成工程并构建（本机需 Xcode）

```bash
# 仓库根
make mobile-ios          # xcodegen + xcodebuild (iOS Simulator)
# 或分步：
cd apps/mobile-ios
xcodegen generate
xcodebuild \
  -project EarningsSync.xcodeproj \
  -scheme EarningsSync \
  -destination 'platform=iOS Simulator,name=iPhone 16' \
  -configuration Debug \
  CODE_SIGNING_ALLOWED=NO \
  build
```

打开 Xcode：`open apps/mobile-ios/EarningsSync.xcodeproj`，运行前确保 API 在 `:8090`：

```bash
AUTH_MODE=bypass STORAGE_MODE=dual \
  DATABASE_URL=postgres://earnings:earnings@127.0.0.1:5432/earnings?sslmode=disable \
  go run ./cmd/server -conf trpc_go.yaml
```

模拟器访问本机 API 用 `http://127.0.0.1:8090`；真机请改成局域网 IP，并在 Info 允许任意 HTTP（开发骨架默认 ATS 对 localhost 宽松）。

## 不装 Xcode 时

至少可验证共享包：

```bash
make mobile-swift-test   # packages/swift-api-client
```

`EarningsSync.xcodeproj` 由 `project.yml` 生成；已生成的工程可提交，便于无 xcodegen 的机器直接 `xcodebuild`。
