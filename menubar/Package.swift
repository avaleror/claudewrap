// swift-tools-version: 6.0
import PackageDescription

let package = Package(
    name: "ClaudeWrap",
    platforms: [.macOS(.v14)],
    targets: [
        .executableTarget(
            name: "ClaudeWrap",
            path: "Sources/ClaudeWrapMenuBar"
        )
    ]
)
