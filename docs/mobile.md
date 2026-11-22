# 多端原生骨架可构建说明

客户端统一读 HTTP 契约（`AUTH_MODE=bypass` 可无密钥演示）：

- `GET /v1/meta` → `storage.{want_mode,mode,sheets_kind,pg_kind}`
- `GET /v1/facts?ticker=`
- `GET /v1/platform/status`

## iOS（已在本仓库验证可 Simulator 构建）

依赖：Xcode 16+、[`xcodegen`](https://github.com/yonaskolb/XcodeGen)（生成工程；也可直接使用已提交的 `EarningsSync.xcodeproj`）。

```bash
make mobile-swift-test    # packages/swift-api-client
make mobile-ios           # xcodegen generate + xcodebuild (iPhone 16 Simulator)
```

详情：[apps/mobile-ios/README.md](../apps/mobile-ios/README.md)。

## Android

依赖：JDK 17、Android SDK（`compileSdk 35`）、网络拉 Gradle 插件。

```bash
make mobile-android
# 等价：cd apps/mobile-android && ./gradlew :app:assembleDebug
```

若本机无 JDK，`make monorepo-check` 仍会校验源码与 Gradle 文件齐全；完整 APK 构建在有 SDK 的机器上执行。模拟器访问宿主机 API 默认 `http://10.0.2.2:8090`。

详情：[apps/mobile-android/README.md](../apps/mobile-android/README.md)。

## 与 Web 的关系

| 客户端 | 共享包 |
| --- | --- |
| `apps/web` | `packages/api-client` |
| `apps/mobile-ios` | `packages/swift-api-client` |
| `apps/mobile-android` | 应用内 `ApiClient.kt`（同形状 JSON） |
