// swift-tools-version: 5.9
import PackageDescription

let package = Package(
    name: "EarningsSyncAPI",
    platforms: [
        .macOS(.v13),
        .iOS(.v16),
    ],
    products: [
        .library(name: "EarningsSyncAPI", targets: ["EarningsSyncAPI"]),
    ],
    targets: [
        .target(name: "EarningsSyncAPI"),
        .testTarget(
            name: "EarningsSyncAPITests",
            dependencies: ["EarningsSyncAPI"]
        ),
    ]
)
