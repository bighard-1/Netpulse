import Foundation

struct HistoryQueryPlan {
    let start: Date
    let end: Date
    let interval: String
    let maxPoints: Int
}

enum TimePreset: String, CaseIterable {
    case day
    case sevenDays
    case thirtyDays
    case oneYear
    case threeYears
}

enum HistoryQueryPlanner {
    static func plan(_ preset: TimePreset, now: Date = Date()) -> HistoryQueryPlan {
        switch preset {
        case .day:
            return .init(start: Calendar.current.startOfDay(for: now), end: now, interval: "1m", maxPoints: 1440)
        case .sevenDays:
            let start = floorToMinute(Calendar.current.date(byAdding: .day, value: -7, to: now) ?? now, minuteStep: 5)
            return .init(start: start, end: now, interval: "5m", maxPoints: 2200)
        case .thirtyDays:
            let start = floorToHour(Calendar.current.date(byAdding: .day, value: -30, to: now) ?? now)
            return .init(start: start, end: now, interval: "1h", maxPoints: 1800)
        case .oneYear:
            let start = floorToHour(Calendar.current.date(byAdding: .year, value: -1, to: now) ?? now, hourStep: 6)
            return .init(start: start, end: now, interval: "6h", maxPoints: 1400)
        case .threeYears:
            let start = floorToHour(Calendar.current.date(byAdding: .year, value: -3, to: now) ?? now, hourStep: 24)
            return .init(start: start, end: now, interval: "1h", maxPoints: 1200)
        }
    }

    private static func floorToMinute(_ date: Date, minuteStep: Int) -> Date {
        var c = Calendar.current.dateComponents([.year, .month, .day, .hour, .minute], from: date)
        let m = c.minute ?? 0
        c.minute = (m / max(1, minuteStep)) * max(1, minuteStep)
        c.second = 0
        return Calendar.current.date(from: c) ?? date
    }

    private static func floorToHour(_ date: Date, hourStep: Int = 1) -> Date {
        var c = Calendar.current.dateComponents([.year, .month, .day, .hour], from: date)
        let h = c.hour ?? 0
        c.hour = (h / max(1, hourStep)) * max(1, hourStep)
        c.minute = 0
        c.second = 0
        return Calendar.current.date(from: c) ?? date
    }
}
