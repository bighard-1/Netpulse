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
            return .init(start: Calendar.current.date(byAdding: .day, value: -7, to: now) ?? now, end: now, interval: "5m", maxPoints: 2200)
        case .thirtyDays:
            return .init(start: Calendar.current.date(byAdding: .day, value: -30, to: now) ?? now, end: now, interval: "1h", maxPoints: 1800)
        case .oneYear:
            return .init(start: Calendar.current.date(byAdding: .year, value: -1, to: now) ?? now, end: now, interval: "6h", maxPoints: 1400)
        case .threeYears:
            return .init(start: Calendar.current.date(byAdding: .year, value: -3, to: now) ?? now, end: now, interval: "1d", maxPoints: 1200)
        }
    }
}
