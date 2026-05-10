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

    static func readableError(_ raw: String) -> String {
        let s = raw.lowercased()
        if s.contains("code=-1009") || s.contains("offline") || s.contains("denied over wi-fi") {
            return "网络不可用或当前网络策略拒绝访问，请检查Wi-Fi/蜂窝网络与服务器地址后重试。"
        }
        if s.contains("code=-1001") || s.contains("timed out") {
            return "请求超时，请检查服务器状态或网络质量。"
        }
        if s.contains("401") || s.contains("unauthorized") || s.contains("invalid token") {
            return "登录已失效，请重新登录。"
        }
        if s.contains("could not connect") || s.contains("failed to connect") {
            return "无法连接服务器，请确认地址与端口可达。"
        }
        return "请求失败，请稍后重试。"
    }
}
