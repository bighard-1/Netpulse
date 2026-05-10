import Foundation

@MainActor
final class PortDetailViewModel: ObservableObject {
    @Published var state: ViewState<TrafficRenderModel> = .loading
    @Published var preset: TimePreset = .day
    @Published var loadingText = ""
    @Published var customStart: Date = Calendar.current.date(byAdding: .day, value: -1, to: Date()) ?? Date()
    @Published var customEnd: Date = Date()

    private var task: Task<Void, Never>?

    func load(portID: String) {
        let plan = HistoryQueryPlanner.plan(preset)
        loadWithPlan(portID: portID, plan: plan)
    }

    func loadCustom(portID: String) {
        let now = Date()
        let minStart = Calendar.current.date(byAdding: .year, value: -3, to: now) ?? now
        let rawStart = max(customStart, minStart)
        let rawEnd = min(customEnd, now)
        guard rawEnd > rawStart else {
            state = .error("结束时间必须晚于开始时间")
            return
        }

        let span = rawEnd.timeIntervalSince(rawStart)
        let (interval, maxPoints, alignedStart, alignedEnd): (String, Int, Date, Date)

        switch Int(span) {
        case ..<86400:
            interval = "1m"
            maxPoints = 1800
            alignedStart = floorToMinute(rawStart, step: 1)
            alignedEnd = floorToMinute(rawEnd, step: 1)
        case ..<691200:
            interval = "5m"
            maxPoints = 2200
            alignedStart = floorToMinute(rawStart, step: 5)
            alignedEnd = floorToMinute(rawEnd, step: 5)
        default:
            interval = "1h"
            maxPoints = 2200
            alignedStart = floorToHour(rawStart, step: 1)
            alignedEnd = floorToHour(rawEnd, step: 1)
        }

        let plan = HistoryQueryPlan(start: alignedStart, end: alignedEnd, interval: interval, maxPoints: maxPoints)
        loadWithPlan(portID: portID, plan: plan)
    }

    private func loadWithPlan(portID: String, plan: HistoryQueryPlan) {
        task?.cancel()
        task = Task {
            state = .loading
            loadingText = "正在查询流量数据..."
            let client = APIClient(baseURL: UserDefaults.standard.string(forKey: "np.base") ?? "http://119.40.55.18:18080/api", token: KeychainManager.loadToken())
            do {
                let points = try await client.fetchTraffic(id: portID, start: plan.start, end: plan.end, maxPoints: plan.maxPoints, interval: plan.interval)
                if Task.isCancelled { return }
                loadingText = "正在处理图表..."
                let model = await ChartDataProcessor.buildTrafficModel(points: points)
                if Task.isCancelled { return }
                state = .success(model)
            } catch {
                if Task.isCancelled { return }
                state = .error(String(describing: error))
            }
            loadingText = ""
        }
    }

    private func floorToMinute(_ date: Date, step: Int) -> Date {
        var c = Calendar.current.dateComponents([.year, .month, .day, .hour, .minute], from: date)
        let m = c.minute ?? 0
        c.minute = (m / max(1, step)) * max(1, step)
        c.second = 0
        return Calendar.current.date(from: c) ?? date
    }

    private func floorToHour(_ date: Date, step: Int) -> Date {
        var c = Calendar.current.dateComponents([.year, .month, .day, .hour], from: date)
        let h = c.hour ?? 0
        c.hour = (h / max(1, step)) * max(1, step)
        c.minute = 0
        c.second = 0
        return Calendar.current.date(from: c) ?? date
    }
}
