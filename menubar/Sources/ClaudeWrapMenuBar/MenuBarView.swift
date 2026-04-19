import SwiftUI

struct MenuBarView: View {
    @ObservedObject var monitor: TokenMonitor

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Text("Claude Session")
                    .font(.headline)
                Spacer()
                if monitor.snapshot.activeSessions > 1 {
                    Text("\(monitor.snapshot.activeSessions) sessions")
                        .font(.caption)
                        .foregroundColor(.secondary)
                }
            }

            // Token bar
            let pct = monitor.snapshot.remainingPct
            VStack(alignment: .leading, spacing: 2) {
                ProgressView(value: pct / 100)
                    .accentColor(barColor(pct))
                HStack {
                    Text(String(format: "%.0f%% remaining", pct))
                        .font(.caption)
                        .foregroundColor(barColor(pct))
                    Spacer()
                    Text(resetLabel)
                        .font(.caption)
                        .foregroundColor(.secondary)
                }
            }

            if monitor.snapshot.isPeak {
                Text("Peak hours — throttle active")
                    .font(.caption2)
                    .foregroundColor(.orange)
            }

            // Compaction
            let cc = monitor.snapshot.compactionCount
            if cc > 0 {
                Text(compactionLabel(cc))
                    .font(.caption)
                    .foregroundColor(cc >= 3 ? .red : cc >= 2 ? .orange : .secondary)
            }

            // Fallback
            if !monitor.snapshot.fallbackEngine.isEmpty {
                Divider()
                Text("AI Fallback: \(monitor.snapshot.fallbackEngine)")
                    .font(.caption)
                Text(String(format: "Cost today: $%.4f", monitor.snapshot.fallbackCost))
                    .font(.caption)
                    .foregroundColor(.secondary)
            }

            Divider()
            Button("Quit ClaudeWrap") { NSApplication.shared.terminate(nil) }
                .font(.caption)
        }
        .padding(12)
        .frame(width: 260)
    }

    private var resetLabel: String {
        guard let reset = monitor.snapshot.estimatedReset else { return "unknown" }
        let remaining = reset.timeIntervalSinceNow
        if remaining <= 0 { return "resetting..." }
        let h = Int(remaining / 3600)
        let m = Int(remaining.truncatingRemainder(dividingBy: 3600) / 60)
        return String(format: "~%dh %02dm (est.)", h, m)
    }

    private func barColor(_ pct: Double) -> Color {
        if pct <= 11 { return .red }
        if pct <= 30 { return .orange }
        return .green
    }

    private func compactionLabel(_ count: Int) -> String {
        switch count {
        case 3...: return "Compacted \(count)x — restart session"
        case 2: return "Compacted 2x — quality degrading"
        default: return "Compacted \(count)x"
        }
    }
}
