import Foundation

enum Fmt {
    static let isoFrac: ISO8601DateFormatter = {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return f
    }()

    static let iso: ISO8601DateFormatter = {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime]
        return f
    }()

    static func bps(_ value: Double) -> String {
        let absV = abs(value)
        if absV >= 1_000_000_000 { return String(format: "%.1f Gbps", value / 1_000_000_000) }
        if absV >= 1_000_000 { return String(format: "%.1f Mbps", value / 1_000_000) }
        if absV >= 1_000 { return String(format: "%.1f Kbps", value / 1_000) }
        return String(format: "%.0f bps", value)
    }

    static func status(_ s: String) -> String {
        switch s.lowercased() {
        case "online", "up": return "在线"
        case "offline", "down": return "离线"
        default: return "未知"
        }
    }
}
