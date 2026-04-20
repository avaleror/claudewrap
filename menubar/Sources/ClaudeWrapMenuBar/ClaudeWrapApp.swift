import SwiftUI
import UserNotifications

@main
struct ClaudeWrapApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) var delegate

    var body: some Scene {
        Settings { EmptyView() }
    }
}

@MainActor
class AppDelegate: NSObject, NSApplicationDelegate, UNUserNotificationCenterDelegate {
    var statusItem: NSStatusItem?
    var popover: NSPopover?
    let monitor = TokenMonitor()
    var iconUpdateTimer: Timer?

    func applicationDidFinishLaunching(_ notification: Notification) {
        NSApp.setActivationPolicy(.accessory)

        UNUserNotificationCenter.current().delegate = self
        UNUserNotificationCenter.current().requestAuthorization(options: [.alert, .sound]) { _, _ in }

        statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
        if let btn = statusItem?.button {
            btn.title = "CC"
            btn.action = #selector(togglePopover)
            btn.target = self
        }

        let view = MenuBarView(monitor: monitor)
        popover = NSPopover()
        popover?.contentViewController = NSHostingController(rootView: view)
        popover?.behavior = .transient

        monitor.start()

        iconUpdateTimer = Timer.scheduledTimer(withTimeInterval: 5, repeats: true) { [weak self] _ in
            Task { @MainActor in self?.updateIcon() }
        }
        updateIcon()

        // Watch for low-token events
        NotificationCenter.default.addObserver(
            self, selector: #selector(onSnapshotUpdate),
            name: .tokenSnapshotUpdated, object: nil
        )
    }

    @objc func togglePopover(_ sender: AnyObject?) {
        guard let btn = statusItem?.button else { return }
        if popover?.isShown == true {
            popover?.performClose(nil)
        } else {
            popover?.show(relativeTo: btn.bounds, of: btn, preferredEdge: .minY)
        }
    }

    @objc func onSnapshotUpdate() {
        let snap = monitor.snapshot
        if snap.remainingPct <= 11 {
            sendNotification(
                title: "ClaudeWrap — Low tokens",
                body: String(format: "%.0f%% remaining. Resets %@", snap.remainingPct, resetLabel(snap))
            )
        }
    }

    func updateIcon() {
        let snap = monitor.snapshot
        if snap.remainingPct <= 11 {
            statusItem?.button?.title = "CC⚠"
        } else {
            statusItem?.button?.title = String(format: "CC %.0f%%", snap.remainingPct)
        }
    }

    func sendNotification(title: String, body: String) {
        let content = UNMutableNotificationContent()
        content.title = title
        content.body = body
        content.sound = .default
        let req = UNNotificationRequest(
            identifier: UUID().uuidString,
            content: content,
            trigger: nil
        )
        UNUserNotificationCenter.current().add(req)
    }

    private func resetLabel(_ snap: TokenSnapshot) -> String {
        guard let reset = snap.estimatedReset else { return "soon" }
        let rem = reset.timeIntervalSinceNow
        guard rem > 0 else { return "now" }
        let h = Int(rem / 3600)
        let m = Int(rem.truncatingRemainder(dividingBy: 3600) / 60)
        return String(format: "in ~%dh %02dm", h, m)
    }
}

extension Notification.Name {
    static let tokenSnapshotUpdated = Notification.Name("tokenSnapshotUpdated")
}
