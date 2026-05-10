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
        let maxStart = Calendar.current.date(byAdding: .year, value: -3, to: now) ?? now
        let s = max(customStart, maxStart)
        let e = min(customEnd, now)
        guard e > s else {
            state = .error("结束时间必须晚于开始时间")
            return
        }
        // 自定义区间交给后端按点数自动聚合。
        let plan = HistoryQueryPlan(start: s, end: e, interval: "1h", maxPoints: 2200)
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
}
