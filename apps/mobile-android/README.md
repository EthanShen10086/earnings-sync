# apps/mobile-android

原生 Android（Kotlin + Compose）骨架：调用同一 HTTP 契约（`/v1/meta`、`/v1/facts`），展示 Sheets/PG 并存状态。

## 构建（需 JDK 17 + Android SDK）

```bash
cd apps/mobile-android
./gradlew :app:assembleDebug
# 或从仓库根：
make mobile-android
```

安装到模拟器/设备：

```bash
./gradlew :app:installDebug
adb shell am start -n com.earningssync.mobile/.MainActivity
```

模拟器访问宿主机 API：`http://10.0.2.2:8090`（已作为默认 Base URL）。

## 无 JDK / SDK 时

`make monorepo-check` 只校验目录与 Gradle 文件存在，不编译 APK。完整构建说明见 [`docs/mobile.md`](../../docs/mobile.md)。
