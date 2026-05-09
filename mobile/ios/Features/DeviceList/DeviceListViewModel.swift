import Foundation

@MainActor
final class DeviceListViewModel: ObservableObject {
    @Published var state: ViewState<[Device]> = .loading
    @Published var keyword = ""
    @Published var results: [SearchResult] = []
    @Published var searching = false

    private var searchTask: Task<Void, Never>?

    private var client: APIClient {
        APIClient(baseURL: UserDefaults.standard.string(forKey: "np.base") ?? "http://119.40.55.18:18080/api", token: KeychainManager.loadToken())
    }

    func load() async {
        state = .loading
        do {
            state = .success(try await client.fetchDevices())
        } catch {
            state = .error(String(describing: error))
        }
    }

    func onKeywordChanged(devices: [Device]) {
        searchTask?.cancel()
        let keyword = self.keyword
        searchTask = Task {
            try? await Task.sleep(nanoseconds: 450_000_000)
            if Task.isCancelled { return }
            let k = keyword.trimmingCharacters(in: .whitespacesAndNewlines)
            guard !k.isEmpty else {
                await MainActor.run {
                    self.searching = false
                    self.results = []
                }
                return
            }

            await MainActor.run { self.searching = true }

            // 优先使用后端全局搜索（可直达端口）；失败时回退本地模糊。
            do {
                let remote = try await client.search(keyword: k)
                if Task.isCancelled { return }
                let mapped = remote.compactMap { item -> SearchResult? in
                    if item.type == "port", let did = item.device_id, let pid = item.interface_id {
                        let name = (item.interface_custom_name?.isEmpty == false ? item.interface_custom_name! : (item.interface_name ?? "端口"))
                        let sub = "\(item.device_name ?? item.device_ip ?? "设备") · \(item.device_ip ?? "")"
                        return SearchResult(category: .port, title: name, subtitle: sub, deviceID: String(did), portID: String(pid))
                    }
                    if let did = item.device_id {
                        let title = item.device_name?.isEmpty == false ? item.device_name! : (item.device_ip ?? "设备")
                        let sub = item.device_ip ?? ""
                        return SearchResult(category: .device, title: title, subtitle: sub, deviceID: String(did), portID: nil)
                    }
                    return nil
                }
                await MainActor.run {
                    self.searching = false
                    self.results = Array(mapped.prefix(50))
                }
                return
            } catch {
                // fallback
            }

            if Task.isCancelled { return }
            let kk = k.lowercased()
            let local: [SearchResult] = devices.flatMap { d in
                d.interfaces.compactMap { p in
                    let title = (p.custom_name?.isEmpty == false ? p.custom_name! : p.name)
                    let blob = "\(d.name) \(d.ip) \(d.remark) \(title) \(p.remark)".lowercased()
                    guard blob.contains(kk) else { return nil }
                    return SearchResult(category: .port, title: title, subtitle: d.name.isEmpty ? d.ip : d.name, deviceID: String(d.id), portID: String(p.id))
                }
            }
            await MainActor.run {
                self.searching = false
                self.results = Array(local.prefix(30))
            }
        }
    }
}
