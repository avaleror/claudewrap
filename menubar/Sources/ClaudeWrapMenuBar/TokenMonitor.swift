import Foundation

struct TokenSnapshot {
    var remainingPct: Double
    var usedTokens: Int
    var totalTokens: Int
    var estimatedReset: Date?
    var compactionCount: Int
    var isPeak: Bool
    var fallbackEngine: String
    var fallbackCost: Double
    var activeSessions: Int
}

@MainActor
final class TokenMonitor: ObservableObject {
    @Published var snapshot = TokenSnapshot(
        remainingPct: 100, usedTokens: 0, totalTokens: 0,
        estimatedReset: nil, compactionCount: 0, isPeak: false,
        fallbackEngine: "", fallbackCost: 0, activeSessions: 0
    )

    private var timer: Timer?
    private let projectsURL: URL
    private let sessionDir: URL

    nonisolated init() {
        let home = FileManager.default.homeDirectoryForCurrentUser
        projectsURL = home.appendingPathComponent(".claude/projects")
        sessionDir = home.appendingPathComponent(".claudewrap/sessions")
    }

    func start() {
        timer = Timer.scheduledTimer(withTimeInterval: 2.0, repeats: true) { [weak self] _ in
            Task { @MainActor in self?.refresh() }
        }
        refresh()
    }

    func stop() {
        timer?.invalidate()
    }

    private func refresh() {
        var snap = TokenSnapshot(
            remainingPct: 100, usedTokens: 0, totalTokens: 0,
            estimatedReset: nil, compactionCount: 0, isPeak: isPeakHour(),
            fallbackEngine: "", fallbackCost: 0, activeSessions: 0
        )

        let jsonlFiles = findActiveJSONLFiles()
        snap.activeSessions = jsonlFiles.count

        for url in jsonlFiles {
            if let s = parseJSONL(url) {
                snap.usedTokens += s.usedTokens
                snap.totalTokens = max(snap.totalTokens, s.totalTokens)
                snap.remainingPct = min(snap.remainingPct, s.remainingPct)
                if snap.estimatedReset == nil {
                    snap.estimatedReset = s.estimatedReset
                }
            }
        }

        snap.compactionCount = readCompactionCount()

        snapshot = snap
    }

    private func findActiveJSONLFiles() -> [URL] {
        guard let enumerator = FileManager.default.enumerator(
            at: projectsURL,
            includingPropertiesForKeys: [.contentModificationDateKey],
            options: [.skipsHiddenFiles]
        ) else { return [] }

        let cutoff = Date().addingTimeInterval(-5 * 3600)
        var files: [URL] = []

        for case let url as URL in enumerator {
            guard url.pathExtension == "jsonl" else { continue }
            let attrs = try? url.resourceValues(forKeys: [.contentModificationDateKey])
            if let mod = attrs?.contentModificationDate, mod > cutoff {
                files.append(url)
            }
        }
        return files
    }

    private struct ParsedSession {
        var remainingPct: Double
        var usedTokens: Int
        var totalTokens: Int
        var estimatedReset: Date?
    }

    private func parseJSONL(_ url: URL) -> ParsedSession? {
        guard let data = try? Data(contentsOf: url),
              let content = String(data: data, encoding: .utf8) else { return nil }

        let lines = content.components(separatedBy: "\n").filter { !$0.isEmpty }
        var firstTimestamp: Date?
        var latest = ParsedSession(remainingPct: 100, usedTokens: 0, totalTokens: 0)

        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601

        for line in lines {
            guard let lineData = line.data(using: .utf8),
                  let obj = try? JSONSerialization.jsonObject(with: lineData) as? [String: Any]
            else { continue }

            if firstTimestamp == nil, let ts = obj["timestamp"] as? String {
                let fmt = ISO8601DateFormatter()
                firstTimestamp = fmt.date(from: ts)
            }

            if let msg = obj["message"] as? [String: Any],
               let usage = msg["usage"] as? [String: Any] {
                if let rem = usage["remaining_percentage"] as? Double {
                    latest.remainingPct = rem
                }
                if let inp = usage["input_tokens"] as? Int,
                   let out = usage["output_tokens"] as? Int {
                    latest.usedTokens = inp + out
                }
                if let cw = usage["context_window"] as? [String: Any],
                   let total = cw["total_tokens"] as? Int {
                    latest.totalTokens = total
                }
            }
        }

        if let first = firstTimestamp {
            latest.estimatedReset = estimatedReset(from: first)
        }
        return latest
    }

    private func estimatedReset(from first: Date) -> Date {
        let hours: Double = isPeakHour() ? 5.0 / 1.75 : 5.0
        return first.addingTimeInterval(hours * 3600)
    }

    private func isPeakHour() -> Bool {
        var cal = Calendar.current
        cal.timeZone = TimeZone(identifier: "America/Los_Angeles")!
        let hour = cal.component(.hour, from: Date())
        return hour >= 5 && hour < 11
    }

    private func readCompactionCount() -> Int {
        let home = FileManager.default.homeDirectoryForCurrentUser
        let f = home.appendingPathComponent(".claudewrap/current-session")
        guard let id = try? String(contentsOf: f, encoding: .utf8).trimmingCharacters(in: .whitespacesAndNewlines),
              !id.isEmpty else { return 0 }
        let sessionFile = sessionDir.appendingPathComponent("\(id).json")
        guard let data = try? Data(contentsOf: sessionFile),
              let obj = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
              let count = obj["compaction_count"] as? Int else { return 0 }
        return count
    }
}
