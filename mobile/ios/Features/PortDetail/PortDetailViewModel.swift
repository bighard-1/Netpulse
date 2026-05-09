import Foundation

@MainActor
final class PortDetailViewModel: ObservableObject {
    @Published var state: ViewState<TrafficRenderModel> = .loading
    @Published var preset: TimePreset = .day
    @Published var loadingText = ""

    private var task: Task<Void, Never>?

    func load(portID: String) {
        task?.cancel()
        task = Task {
            state = .loading
            loadingText = "正在查询流量数据..."
            let plan = HistoryQueryPlanner.plan(preset)
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
