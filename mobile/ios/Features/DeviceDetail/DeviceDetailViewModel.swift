import Foundation

@MainActor
final class DeviceDetailViewModel: ObservableObject {
    @Published var state: ViewState<Device> = .loading
    @Published var cpu: [DeviceHistoryPoint] = []
    @Published var mem: [DeviceHistoryPoint] = []
    @Published var loadingHistory = false

    private var task: Task<Void, Never>?
    private var historyTask: Task<Void, Never>?

    private var client: APIClient {
        APIClient(baseURL: UserDefaults.standard.string(forKey: "np.base") ?? "http://119.40.55.18:18080/api", token: KeychainManager.loadToken())
    }

    func load(deviceID: String) {
        task?.cancel()
        historyTask?.cancel()

        task = Task {
            state = .loading
            do {
                let d = try await client.fetchDevice(id: deviceID)
                if Task.isCancelled { return }
                state = .success(d)
                loadHistory(deviceID: deviceID)
            } catch {
                if Task.isCancelled { return }
                state = .error(String(describing: error))
            }
        }
    }

    func loadHistory(deviceID: String) {
        historyTask?.cancel()
        historyTask = Task {
            loadingHistory = true
            defer { loadingHistory = false }
            do {
                let start = Calendar.current.startOfDay(for: Date())
                async let c = client.fetchDeviceHistory(type: "cpu", id: deviceID, start: start, end: Date(), maxPoints: 1440, interval: "1m")
                async let m = client.fetchDeviceHistory(type: "mem", id: deviceID, start: start, end: Date(), maxPoints: 1440, interval: "1m")
                let (cpuData, memData) = try await (c, m)
                if Task.isCancelled { return }
                cpu = cpuData
                mem = memData
            } catch {
                if Task.isCancelled { return }
                cpu = []
                mem = []
            }
        }
    }
}
