import SwiftUI
import LocalAuthentication
import Charts
import Security
import UIKit

enum UiSpec {
    static let pagePadding: CGFloat = 16
    static let sectionGap: CGFloat = 12
    static let cardRadius: CGFloat = 12
}

enum NpColor {
    static let bg = Color(red: 15/255, green: 23/255, blue: 42/255)
    static let indigo = Color(red: 99/255, green: 102/255, blue: 241/255)
    static let card = Color(red: 30/255, green: 41/255, blue: 59/255)
    static let success = Color(red: 16/255, green: 185/255, blue: 129/255)
    static let danger = Color(red: 239/255, green: 68/255, blue: 68/255)
    static let warning = Color(red: 245/255, green: 158/255, blue: 11/255)
}

struct DeviceStatus: Codable, Identifiable {
    let id: Int64
    let ip: String
    let name: String
    let brand: String
    let community: String?
    let remark: String
    let created_at: String
    let last_metric_at: String?
    let status: String
    let maintenance_mode: Bool?
    let status_reason: String?
    let interfaces: [NetInterface]
}

struct NetInterface: Codable, Identifiable {
    let id: Int64
    let device_id: Int64?
    let index: Int
    let name: String
    let remark: String
}

struct DeviceLog: Codable, Identifiable {
    let id: Int64
    let device_id: Int64
    let level: String
    let message: String
    let created_at: String
}

struct RecentEvent: Codable, Identifiable {
    let id: Int64
    let device_id: Int64?
    let device_name: String?
    let level: String?
    let message: String?
    let created_at: String?
}

struct DeviceNavTarget: Hashable {
    let id: Int64
}

struct PortNavTarget: Hashable {
    let id: Int64
    let deviceID: Int64
}

struct MetricLinePoint: Identifiable {
    let id: String
    let ts: Date
    let value: Double
}

enum UsageKind {
    case cpu
    case mem
}

struct UsageLinePoint: Identifiable {
    let id: String
    let ts: Date
    let value: Double
    let kind: UsageKind
}

struct DeviceHistoryPoint: Codable, Identifiable {
    var id: String { timestamp }
    let timestamp: String
    let cpu_usage: Double?
    let mem_usage: Double?
}

struct InterfaceHistoryPoint: Codable, Identifiable {
    var id: String { timestamp }
    let timestamp: String
    let traffic_in_bps: Double?
    let traffic_out_bps: Double?
}

struct HistoryResp<T: Codable>: Codable {
    let type: String
    let id: Int64
    let data: [T]
}

struct LoginResponse: Codable {
    struct U: Codable { let username: String; let role: String }
    let token: String
    let user: U
}

@MainActor
final class AppVM: ObservableObject {
    @Published var token: String = KeychainStore.shared.get("netpulse_jwt") ?? ""
    @Published var devices: [DeviceStatus] = []
    @Published var recentEvents: [RecentEvent] = []
    @Published var deviceDetail: DeviceStatus?
    @Published var logs: [DeviceLog] = []
    @Published var cpu: [DeviceHistoryPoint] = []
    @Published var mem: [DeviceHistoryPoint] = []
    @Published var traffic: [InterfaceHistoryPoint] = []
    @Published var loading = false
    @Published var detailLoading = false
    @Published var historyLoading = false
    @Published var portLoading = false
    @Published var err = ""
    @Published var loginError = ""
    @Published var dashboardError = ""
    @Published var deviceError = ""
    @Published var portError = ""

    private var detailRequestSeq: Int = 0
    private var historyRequestSeq: Int = 0
    private var portRequestSeq: Int = 0
    private var activeDeviceID: Int64?
    private var activePortID: Int64?

    var baseURL: String {
        get { UserDefaults.standard.string(forKey: "baseURL") ?? "http://119.40.55.18:18080/api" }
        set { UserDefaults.standard.set(newValue, forKey: "baseURL") }
    }

    private func authorizedRequest(_ url: URL) -> URLRequest {
        var req = URLRequest(url: url)
        if !token.isEmpty { req.addValue("Bearer \(token)", forHTTPHeaderField: "Authorization") }
        return req
    }

    private func dataWithAuth(_ req: URLRequest) async throws -> Data {
        let (data, resp) = try await URLSession.shared.data(for: req)
        if let http = resp as? HTTPURLResponse, http.statusCode == 401 {
            await MainActor.run {
                let msg = "登录已过期，请重新登录"
                err = msg
                loginError = msg
                logout()
            }
            throw NSError(domain: "auth", code: 401)
        }
        return data
    }

    func login(u: String, p: String, remember: Bool = true) async {
        loading = true
        defer { loading = false }
        do {
            let url = URL(string: "\(baseURL)/auth/mobile/login")!
            var req = URLRequest(url: url)
            req.httpMethod = "POST"
            req.addValue("application/json", forHTTPHeaderField: "Content-Type")
            req.httpBody = try JSONSerialization.data(withJSONObject: ["username": u, "password": p])
            let (data, resp) = try await URLSession.shared.data(for: req)
            guard let h = resp as? HTTPURLResponse else { throw NSError(domain: "login", code: 1) }
            guard (200..<300).contains(h.statusCode) else {
                if let obj = try? JSONSerialization.jsonObject(with: data) as? [String: Any] {
                    let m = (obj["error"] as? String) ?? (obj["message"] as? String) ?? "登录失败"
                    throw NSError(domain: "login", code: h.statusCode, userInfo: [NSLocalizedDescriptionKey: m])
                }
                throw NSError(domain: "login", code: h.statusCode, userInfo: [NSLocalizedDescriptionKey: "登录失败(\(h.statusCode))"])
            }
            let r = try JSONDecoder().decode(LoginResponse.self, from: data)
            token = r.token
            KeychainStore.shared.set("netpulse_jwt", value: r.token)
            loginError = ""
            await refreshDevices()
        } catch {
            let msg = (error as NSError).localizedDescription
            let m = "登录失败：\(msg)"
            err = m
            loginError = m
        }
    }

    func biometricLogin() async {
        let ctx = LAContext()
        var e: NSError?
        guard ctx.canEvaluatePolicy(.deviceOwnerAuthenticationWithBiometrics, error: &e) else { return }
        if (try? await ctx.evaluatePolicy(.deviceOwnerAuthenticationWithBiometrics, localizedReason: "用于快速登录 NetPulse")) == true {
            // Biometric auth now only unlocks existing secure token session.
            token = KeychainStore.shared.get("netpulse_jwt") ?? ""
            if token.isEmpty {
                let m = "请先使用用户名密码完成首次登录"
                err = m
                loginError = m
                return
            }
            await refreshDevices()
        }
    }

    func logout() {
        token = ""
        devices = []
        recentEvents = []
        deviceDetail = nil
        cpu = []
        mem = []
        traffic = []
        logs = []
        dashboardError = ""
        deviceError = ""
        portError = ""
        KeychainStore.shared.delete("netpulse_jwt")
    }

    func refreshDevices() async {
        guard !token.isEmpty else { return }
        loading = true
        defer { loading = false }
        do {
            let req = authorizedRequest(URL(string: "\(baseURL)/devices")!)
            let d = try await dataWithAuth(req)
            devices = try JSONDecoder().decode([DeviceStatus].self, from: d)
            do {
                recentEvents = try await fetchRecentEvents().prefix(5).map { $0 }
            } catch {
                recentEvents = []
            }
            dashboardError = ""
        } catch {
            let m = "加载设备失败：\((error as NSError).localizedDescription)"
            err = m
            dashboardError = m
        }
    }

    func fetchDeviceDetail(deviceID: Int64) async {
        detailRequestSeq += 1
        let requestID = detailRequestSeq
        activeDeviceID = deviceID
        detailLoading = true
        defer {
            if requestID == detailRequestSeq { detailLoading = false }
        }
        guard !token.isEmpty else { return }
        do {
            let req = authorizedRequest(URL(string: "\(baseURL)/devices/\(deviceID)")!)
            let d = try await dataWithAuth(req)
            guard requestID == detailRequestSeq, activeDeviceID == deviceID else { return }
            deviceDetail = try JSONDecoder().decode(DeviceStatus.self, from: d)
            deviceError = ""
        } catch {
            guard requestID == detailRequestSeq else { return }
            let m = "加载设备详情失败：\((error as NSError).localizedDescription)"
            err = m
            deviceError = m
        }
    }

    func fetchDeviceHistory(deviceID: Int64, start: Date, end: Date) async {
        historyRequestSeq += 1
        let requestID = historyRequestSeq
        activeDeviceID = deviceID
        historyLoading = true
        defer {
            if requestID == historyRequestSeq { historyLoading = false }
        }
        do {
            let s = ISO8601DateFormatter().string(from: start)
            let e = ISO8601DateFormatter().string(from: end)
            async let c = fetchHistory(type: "cpu", id: deviceID, start: s, end: e, maxPoints: 420) as [DeviceHistoryPoint]
            async let m = fetchHistory(type: "mem", id: deviceID, start: s, end: e, maxPoints: 420) as [DeviceHistoryPoint]
            let cpuPoints = try await c
            let memPoints = try await m
            guard requestID == historyRequestSeq, activeDeviceID == deviceID else { return }
            cpu = cpuPoints.sorted { $0.timestamp < $1.timestamp }
            mem = memPoints.sorted { $0.timestamp < $1.timestamp }
            deviceError = ""
            // 日志单独异步加载，避免拖慢详情页首屏图表渲染。
            Task {
                do {
                    let loadedLogs = try await fetchLogs(deviceID: deviceID)
                    guard requestID == self.historyRequestSeq, self.activeDeviceID == deviceID else { return }
                    self.logs = loadedLogs
                } catch {
                    guard requestID == self.historyRequestSeq else { return }
                    self.logs = []
                }
            }
        } catch {
            guard requestID == historyRequestSeq else { return }
            cpu = []
            mem = []
            logs = []
            let m = "加载性能数据失败：\((error as NSError).localizedDescription)"
            err = m
            deviceError = m
        }
    }

    func fetchPortHistory(portID: Int64, start: Date, end: Date) async {
        portRequestSeq += 1
        let requestID = portRequestSeq
        activePortID = portID
        portLoading = true
        defer {
            if requestID == portRequestSeq { portLoading = false }
        }
        do {
            let s = ISO8601DateFormatter().string(from: start)
            let e = ISO8601DateFormatter().string(from: end)
            let points: [InterfaceHistoryPoint] = try await fetchHistory(type: "traffic", id: portID, start: s, end: e, maxPoints: 700)
            guard requestID == portRequestSeq, activePortID == portID else { return }
            traffic = points.sorted { $0.timestamp < $1.timestamp }
            portError = ""
        } catch {
            guard requestID == portRequestSeq else { return }
            traffic = []
            let m = "加载端口流量失败：\((error as NSError).localizedDescription)"
            err = m
            portError = m
        }
    }

    func updateInterfaceRemark(interfaceID: Int64, remark: String, deviceID: Int64, start: Date, end: Date) async {
        do {
            let url = URL(string: "\(baseURL)/interfaces/\(interfaceID)/remark")!
            var req = authorizedRequest(url)
            req.httpMethod = "PUT"
            req.addValue("application/json", forHTTPHeaderField: "Content-Type")
            req.httpBody = try JSONSerialization.data(withJSONObject: ["remark": remark])
            _ = try await dataWithAuth(req)
            await fetchDeviceDetail(deviceID: deviceID)
            await fetchDeviceHistory(deviceID: deviceID, start: start, end: end)
        } catch {
            let m = "更新端口备注失败"
            err = m
            deviceError = m
        }
    }

    func updateDeviceRemark(deviceID: Int64, remark: String) async {
        do {
            let url = URL(string: "\(baseURL)/devices/\(deviceID)/remark")!
            var req = authorizedRequest(url)
            req.httpMethod = "PUT"
            req.addValue("application/json", forHTTPHeaderField: "Content-Type")
            req.httpBody = try JSONSerialization.data(withJSONObject: ["remark": remark])
            _ = try await dataWithAuth(req)
            await refreshDevices()
        } catch {
            let m = "更新设备备注失败"
            err = m
            deviceError = m
        }
    }

    func updateDeviceProfile(deviceID: Int64, name: String, brand: String, remark: String, maintenanceMode: Bool) async {
        do {
            let url = URL(string: "\(baseURL)/devices/\(deviceID)")!
            var req = authorizedRequest(url)
            req.httpMethod = "PUT"
            req.addValue("application/json", forHTTPHeaderField: "Content-Type")
            req.httpBody = try JSONSerialization.data(withJSONObject: [
                "name": name,
                "brand": brand,
                "remark": remark,
                "maintenance_mode": maintenanceMode
            ])
            _ = try await dataWithAuth(req)
            await refreshDevices()
        } catch {
            let m = "更新资产失败"
            err = m
            deviceError = m
        }
    }

    func updateMaintenanceMode(deviceID: Int64, enabled: Bool) async {
        do {
            let dev: DeviceStatus
            if let cached = deviceDetail {
                dev = cached
            } else {
                let req = authorizedRequest(URL(string: "\(baseURL)/devices/\(deviceID)")!)
                let d = try await dataWithAuth(req)
                dev = try JSONDecoder().decode(DeviceStatus.self, from: d)
            }
            let url = URL(string: "\(baseURL)/devices/\(deviceID)")!
            var req = authorizedRequest(url)
            req.httpMethod = "PUT"
            req.addValue("application/json", forHTTPHeaderField: "Content-Type")
            req.httpBody = try JSONSerialization.data(withJSONObject: [
                "name": dev.name,
                "brand": dev.brand,
                "remark": dev.remark,
                "maintenance_mode": enabled
            ])
            _ = try await dataWithAuth(req)
            await fetchDeviceDetail(deviceID: deviceID)
            await refreshDevices()
        } catch {
            let m = "更新维护模式失败"
            err = m
            deviceError = m
        }
    }

    private func fetchLogs(deviceID: Int64) async throws -> [DeviceLog] {
        let req = authorizedRequest(URL(string: "\(baseURL)/devices/\(deviceID)/logs")!)
        let d = try await dataWithAuth(req)
        return try JSONDecoder().decode([DeviceLog].self, from: d)
    }

    private func fetchRecentEvents() async throws -> [RecentEvent] {
        let req = authorizedRequest(URL(string: "\(baseURL)/events/recent?limit=5")!)
        let d = try await dataWithAuth(req)
        if let wrapped = try? JSONDecoder().decode(EventsResponse.self, from: d) {
            return wrapped.data
        }
        return try JSONDecoder().decode([RecentEvent].self, from: d)
    }

    private func fetchHistory<T: Codable>(type: String, id: Int64, start: String, end: String, maxPoints: Int) async throws -> [T] {
        var comp = URLComponents(string: "\(baseURL)/metrics/history")!
        let interval = historyInterval(start: start, end: end)
        comp.queryItems = [
            URLQueryItem(name: "type", value: type),
            URLQueryItem(name: "id", value: "\(id)"),
            URLQueryItem(name: "start", value: start),
            URLQueryItem(name: "end", value: end),
            URLQueryItem(name: "interval", value: interval),
            URLQueryItem(name: "max_points", value: "\(maxPoints)")
        ]
        let req = authorizedRequest(comp.url!)
        let d = try await dataWithAuth(req)
        return try JSONDecoder().decode(HistoryResp<T>.self, from: d).data
    }
}

struct EventsResponse: Codable {
    let data: [RecentEvent]
}

final class KeychainStore {
    static let shared = KeychainStore()
    private init() {}

    func set(_ key: String, value: String) {
        let data = Data(value.utf8)
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrAccount as String: key
        ]
        SecItemDelete(query as CFDictionary)
        let add: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrAccount as String: key,
            kSecValueData as String: data
        ]
        SecItemAdd(add as CFDictionary, nil)
    }

    func get(_ key: String) -> String? {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrAccount as String: key,
            kSecReturnData as String: true,
            kSecMatchLimit as String: kSecMatchLimitOne
        ]
        var item: CFTypeRef?
        let status = SecItemCopyMatching(query as CFDictionary, &item)
        guard status == errSecSuccess, let data = item as? Data else { return nil }
        return String(data: data, encoding: .utf8)
    }

    func delete(_ key: String) {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrAccount as String: key
        ]
        SecItemDelete(query as CFDictionary)
    }
}

@main
struct NetPulseMobileApp: App {
    @StateObject private var vm = AppVM()
    #if DEBUG
    private let skipLoginForDebug = false
    #else
    private let skipLoginForDebug = false
    #endif

    var body: some Scene {
        WindowGroup {
            if !skipLoginForDebug && vm.token.isEmpty {
                LoginView().environmentObject(vm)
            } else {
                MainTabView().environmentObject(vm)
            }
        }
    }
}

struct MainTabView: View {
    @EnvironmentObject var vm: AppVM

    var body: some View {
        TabView {
            DashboardView()
                .environmentObject(vm)
                .tabItem {
                    Label("仪表盘", systemImage: "chart.bar.xaxis")
                }

            AssetCenterView()
                .environmentObject(vm)
                .tabItem {
                    Label("资产中心", systemImage: "rectangle.grid.1x2")
                }

            NavigationStack {
                VStack(spacing: 12) {
                    Text("NetPulse")
                        .font(.title2.bold())
                    Text(vm.baseURL)
                        .font(.footnote)
                        .foregroundStyle(.white.opacity(0.7))
                    Button("退出登录", role: .destructive) { vm.logout() }
                        .buttonStyle(.borderedProminent)
                }
                .padding()
                .frame(maxWidth: .infinity, maxHeight: .infinity)
                .background(NpColor.bg)
            }
            .tabItem {
                Label("我的", systemImage: "person.crop.circle")
            }
        }
        .tint(NpColor.indigo)
    }
}

struct LoginView: View {
    @EnvironmentObject var vm: AppVM
    @State private var u = ""
    @State private var p = ""
    @State private var base = "http://119.40.55.18:18080/api"

    var body: some View {
        VStack(spacing: UiSpec.sectionGap) {
            Text("NetPulse").font(.title.bold())
            Text("移动端只读工作台").foregroundStyle(.white.opacity(0.7))
            TextField("用户名", text: $u).textFieldStyle(.roundedBorder)
            SecureField("密码", text: $p).textFieldStyle(.roundedBorder)
            TextField("服务器 API 地址", text: $base).textFieldStyle(.roundedBorder)
            HStack {
                Button("登录") {
                    vm.baseURL = base
                    Task { await vm.login(u: u, p: p) }
                }
                .buttonStyle(.borderedProminent)
                .tint(NpColor.indigo)

                Button("Face ID / Touch ID") { Task { await vm.biometricLogin() } }
                    .buttonStyle(.bordered)
            }
            Text("首次登录必须使用用户名密码").font(.footnote).foregroundStyle(.white.opacity(0.7))
            if !vm.loginError.isEmpty { Text(vm.loginError).foregroundStyle(.red) }
        }
        .padding(UiSpec.pagePadding)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(NpColor.bg)
    }
}

struct DashboardView: View {
    @EnvironmentObject var vm: AppVM
    @State private var quickPeekDevice: DeviceStatus?

    private var onlineCount: Int { vm.devices.filter { $0.status == "online" }.count }
    private var offlineCount: Int { vm.devices.filter { $0.status != "online" }.count }
    private var criticalCount: Int { vm.recentEvents.filter { severity(of: $0) == .error }.count }
    private var todoItems: [String] {
        var out: [String] = []
        if vm.devices.isEmpty { out.append("添加首台资产（请在 Web 端资产中心操作）") }
        if offlineCount > 0 { out.append("排查离线/未知资产：\(offlineCount) 台") }
        if !vm.recentEvents.isEmpty { out.append("检查最新事件并确认是否需要处置") }
        return out
    }

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(spacing: UiSpec.sectionGap) {
                    HStack {
                        Text("只读模式").font(.caption).padding(.horizontal, 10).padding(.vertical, 4)
                            .background(NpColor.indigo.opacity(0.25))
                            .clipShape(Capsule())
                        Spacer()
                    }
                    .padding(.horizontal, UiSpec.pagePadding)

                    ScrollView(.horizontal, showsIndicators: false) {
                        HStack(spacing: 10) {
                            StatCard(title: "总数", value: "\(vm.devices.count)", gradient: [.slate1, .slate2])
                            StatCard(title: "在线", value: "\(onlineCount)", gradient: [.teal1, .teal2], pulse: true)
                            StatCard(title: "离线", value: "\(offlineCount)", gradient: [.red1, .red2])
                            StatCard(title: "告警", value: "\(criticalCount)", gradient: [.indigo1, .indigo2])
                        }
                        .padding(.horizontal, UiSpec.pagePadding)
                    }

                    if !todoItems.isEmpty {
                        NpCard {
                            Text("今日待处理").font(.headline)
                            ForEach(todoItems.prefix(3), id: \.self) { x in
                                Text("• \(x)")
                                    .font(.footnote)
                                    .foregroundStyle(.white.opacity(0.82))
                            }
                        }
                        .padding(.horizontal, UiSpec.pagePadding)
                    }

                    if vm.devices.isEmpty {
                        EmptyStateCard(title: "暂无资产", desc: "请先在 Web 端添加资产后再查看")
                            .padding(.horizontal, UiSpec.pagePadding)
                    } else {
                        VStack(spacing: 8) {
                            ForEach(vm.devices) { d in
                                NavigationLink(value: DeviceNavTarget(id: d.id)) { DeviceRow(device: d) }
                                    .buttonStyle(.plain)
                                    .contextMenu {
                                        Button("快速预览") { quickPeekDevice = d }
                                    }
                            }
                        }
                        .padding(.horizontal, UiSpec.pagePadding)
                    }

                    NpCard {
                        VStack(alignment: .leading, spacing: 8) {
                            Text("系统实时事件流").font(.headline)
                            if vm.recentEvents.isEmpty {
                                Text("暂无关键事件").foregroundStyle(.white.opacity(0.7))
                            } else {
                                ForEach(vm.recentEvents.prefix(5)) { event in
                                    let sev = severity(of: event)
                                    Text("[\(sev.rawValue.uppercased())] \(event.message ?? "-")")
                                        .font(.footnote)
                                        .foregroundStyle(severityColor(sev))
                                }
                            }
                        }
                    }
                    .padding(.horizontal, UiSpec.pagePadding)
                }
                .padding(.vertical, 8)
            }
            .background(NpColor.bg)
            .navigationTitle("仪表盘").toolbarColorScheme(.dark, for: .navigationBar)
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button("刷新") { Task { await vm.refreshDevices() } }
                }
            }
            .navigationDestination(for: DeviceNavTarget.self) { target in
                DeviceDetailView(deviceID: target.id).environmentObject(vm)
            }
            .task { await vm.refreshDevices() }
            .refreshable { await vm.refreshDevices() }
            .sheet(item: $quickPeekDevice) { d in
                DeviceQuickPeekSheet(device: d)
                    .environmentObject(vm)
                    .presentationDetents([.medium, .large])
            }
        }
    }
}

struct AssetCenterView: View {
    @EnvironmentObject var vm: AppVM
    @State private var quickPeekDevice: DeviceStatus?

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(spacing: UiSpec.sectionGap) {
                    NpCard {
                        HStack {
                            Text("资产中心（只读）").font(.headline)
                            Spacer()
                            Button("刷新") { Task { await vm.refreshDevices() } }
                        }
                    }
                    .padding(.horizontal, UiSpec.pagePadding)

                    if vm.devices.isEmpty {
                        EmptyStateCard(title: "暂无资产", desc: "该页面仅提供查询，新增请使用 Web 端")
                            .padding(.horizontal, UiSpec.pagePadding)
                    } else {
                        VStack(spacing: 8) {
                            ForEach(vm.devices) { d in
                                NavigationLink(value: DeviceNavTarget(id: d.id)) { DeviceRow(device: d) }
                                    .buttonStyle(.plain)
                                    .contextMenu {
                                        Button("快速预览") { quickPeekDevice = d }
                                    }
                            }
                        }
                        .padding(.horizontal, UiSpec.pagePadding)
                    }
                }
                .padding(.vertical, 8)
            }
            .background(NpColor.bg)
            .navigationTitle("资产中心")
            .navigationDestination(for: DeviceNavTarget.self) { target in
                DeviceDetailView(deviceID: target.id).environmentObject(vm)
            }
            .task { await vm.refreshDevices() }
            .refreshable { await vm.refreshDevices() }
            .sheet(item: $quickPeekDevice) { d in
                DeviceQuickPeekSheet(device: d)
                    .environmentObject(vm)
                    .presentationDetents([.medium, .large])
            }
        }
    }
}

struct DeviceDetailView: View {
    @EnvironmentObject var vm: AppVM
    let deviceID: Int64
    @State private var keyword = ""
    @State private var showLogs = false
    @State private var dateEnd = Date()
    @State private var dateStart = Calendar.current.date(byAdding: .day, value: -1, to: Date()) ?? Date()
    @State private var visiblePortCount = 80

    private var filteredPorts: [NetInterface] {
        let list = vm.deviceDetail?.interfaces ?? []
        let key = keyword.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        if key.isEmpty { return list }
        return list.filter { "\($0.id) \($0.index) \($0.name) \($0.remark)".lowercased().contains(key) }
    }
    private var visiblePorts: [NetInterface] { Array(filteredPorts.prefix(visiblePortCount)) }
    private var cpuValues: [Double] { vm.cpu.map { $0.cpu_usage ?? 0 } }
    private var memValues: [Double] { vm.mem.map { $0.mem_usage ?? 0 } }
    private var cpuForRender: [DeviceHistoryPoint] { decimateDeviceHistory(vm.cpu, maxPoints: 420) }
    private var memForRender: [DeviceHistoryPoint] { decimateDeviceHistory(vm.mem, maxPoints: 420) }
    private var usageSeries: [UsageLinePoint] {
        let c = cpuForRender.map { UsageLinePoint(id: "c-\($0.timestamp)", ts: parseRFC3339($0.timestamp), value: $0.cpu_usage ?? 0, kind: .cpu) }
        let m = memForRender.map { UsageLinePoint(id: "m-\($0.timestamp)", ts: parseRFC3339($0.timestamp), value: $0.mem_usage ?? 0, kind: .mem) }
        return (c + m).sorted { $0.ts < $1.ts }
    }
    private var cpuCurrent: Double { cpuValues.last ?? 0 }
    private var memCurrent: Double { memValues.last ?? 0 }
    private var cpuPeak: Double { cpuValues.max() ?? 0 }
    private var memPeak: Double { memValues.max() ?? 0 }

    var body: some View {
        ScrollView {
            VStack(spacing: UiSpec.sectionGap) {
                NpCard {
                    if let d = vm.deviceDetail {
                        HStack {
                            PulseDot(status: d.status)
                            Text(d.name.isEmpty ? d.ip : d.name).font(.headline)
                            Spacer()
                        }
                        Text("\(statusText(d.status)) · \(d.ip) · \(d.brand) · \(d.remark.isEmpty ? "未备注" : d.remark)")
                            .font(.subheadline)
                            .foregroundStyle(.white.opacity(0.7))
                    }
                }

                CpuMemPanel(
                    historyLoading: vm.historyLoading,
                    cpuCurrent: cpuCurrent,
                    cpuPeak: cpuPeak,
                    memCurrent: memCurrent,
                    memPeak: memPeak,
                    cpuSeries: usageSeries.filter { $0.kind == .cpu },
                    memSeries: usageSeries.filter { $0.kind == .mem }
                )

                NpCard {
                    TextField("搜索端口", text: $keyword)
                        .padding(.horizontal, 12)
                        .padding(.vertical, 10)
                        .background(Color.white.opacity(0.08))
                        .clipShape(RoundedRectangle(cornerRadius: 10))
                        .foregroundStyle(.white)
                }

                if vm.detailLoading && (vm.deviceDetail?.interfaces.isEmpty ?? true) {
                    ForEach(0..<3, id: \.self) { _ in ShimmerRect(height: 80) }
                } else if filteredPorts.isEmpty {
                    EmptyStateCard(title: "无匹配端口", desc: "请调整关键字后再试")
                } else {
                    ForEach(visiblePorts) { p in
                        NavigationLink(value: PortNavTarget(id: p.id, deviceID: deviceID)) {
                            NpCard {
                                VStack(alignment: .leading, spacing: 4) {
                                    Text(p.name)
                                    Text("索引:\(p.index) · \(p.remark.isEmpty ? "-" : p.remark)")
                                        .font(.footnote)
                                        .foregroundStyle(.white.opacity(0.7))
                                }
                            }
                        }
                        .buttonStyle(.plain)
                    }
                    if visiblePortCount < filteredPorts.count {
                        Button("加载更多端口（\(filteredPorts.count - visiblePortCount)）") {
                            visiblePortCount += 80
                        }
                        .buttonStyle(.bordered)
                    }
                }

                NpCard {
                    DisclosureGroup(isExpanded: $showLogs) {
                        ForEach(vm.logs.prefix(10)) { log in
                            Text("[\(log.level)] \(log.message)")
                                .font(.footnote)
                                .foregroundStyle(logLevelColor(log.level))
                        }
                    } label: {
                        Text("设备日志（默认10条）").font(.headline.weight(.semibold))
                    }
                }
                if !vm.deviceError.isEmpty {
                    NpCard {
                        Text(vm.deviceError).font(.footnote).foregroundStyle(NpColor.warning)
                    }
                }
            }
            .padding(UiSpec.pagePadding)
        }
        .background(NpColor.bg)
        .navigationTitle("设备详情")
        .navigationBarTitleDisplayMode(.inline)
        .navigationDestination(for: PortNavTarget.self) { target in
            PortDetailView(deviceID: target.deviceID, portID: target.id).environmentObject(vm)
        }
        .task {
            visiblePortCount = 80
            dateEnd = Date()
            dateStart = Calendar.current.date(byAdding: .day, value: -1, to: dateEnd) ?? dateEnd
            async let detailTask: Void = vm.fetchDeviceDetail(deviceID: deviceID)
            async let historyTask: Void = vm.fetchDeviceHistory(deviceID: deviceID, start: dateStart, end: dateEnd)
            _ = await (detailTask, historyTask)
        }
        .refreshable {
            visiblePortCount = 80
            async let detailTask: Void = vm.fetchDeviceDetail(deviceID: deviceID)
            async let historyTask: Void = vm.fetchDeviceHistory(deviceID: deviceID, start: dateStart, end: dateEnd)
            _ = await (detailTask, historyTask)
        }
    }
}

struct PortDetailView: View {
    @EnvironmentObject var vm: AppVM
    let deviceID: Int64
    let portID: Int64
    @State private var start = Calendar.current.date(byAdding: .day, value: -1, to: Date()) ?? Date()
    @State private var end = Date()
    @State private var selectedRange: String = "day"
    @State private var showInSeries = true
    @State private var showOutSeries = true

    private var minDate: Date {
        Calendar.current.date(byAdding: .year, value: -3, to: Date()) ?? .distantPast
    }
    private var trafficForRender: [InterfaceHistoryPoint] {
        decimateTraffic(vm.traffic, maxPoints: 700)
    }

    var body: some View {
        VStack(spacing: UiSpec.sectionGap) {
            NpCard {
                Text("时间范围").font(.headline)
                ScrollView(.horizontal, showsIndicators: false) {
                    HStack(spacing: 8) {
                        Button("当日") {
                            selectedRange = "day"
                            end = Date()
                            start = Calendar.current.startOfDay(for: end)
                            Task { await vm.fetchPortHistory(portID: portID, start: start, end: end) }
                        }.buttonStyle(.bordered)
                        Button("近7天") {
                            selectedRange = "7d"
                            end = Date()
                            start = Calendar.current.date(byAdding: .day, value: -7, to: end) ?? end
                            Task { await vm.fetchPortHistory(portID: portID, start: start, end: end) }
                        }.buttonStyle(.bordered)
                        Button("近30天") {
                            selectedRange = "30d"
                            end = Date()
                            start = Calendar.current.date(byAdding: .day, value: -30, to: end) ?? end
                            Task { await vm.fetchPortHistory(portID: portID, start: start, end: end) }
                        }.buttonStyle(.bordered)
                        Button("近3年") {
                            selectedRange = "3y"
                            end = Date()
                            start = Calendar.current.date(byAdding: .day, value: -365*3, to: end) ?? end
                            Task { await vm.fetchPortHistory(portID: portID, start: start, end: end) }
                        }.buttonStyle(.bordered)
                    }
                }
                DatePicker("开始", selection: $start, in: minDate...Date(), displayedComponents: [.date, .hourAndMinute])
                DatePicker("结束", selection: $end, in: minDate...Date(), displayedComponents: [.date, .hourAndMinute])
                HStack {
                    Button("取消") {
                        end = Date()
                        switch selectedRange {
                        case "7d": start = Calendar.current.date(byAdding: .day, value: -7, to: end) ?? end
                        case "30d": start = Calendar.current.date(byAdding: .day, value: -30, to: end) ?? end
                        case "3y": start = Calendar.current.date(byAdding: .day, value: -365*3, to: end) ?? end
                        default: start = Calendar.current.startOfDay(for: end)
                        }
                    }.buttonStyle(.bordered)
                    Button("查询") { Task { await vm.fetchPortHistory(portID: portID, start: start, end: end) } }
                        .buttonStyle(.borderedProminent)
                        .tint(NpColor.indigo)
                }
            }

            if vm.portLoading {
                ShimmerRect(height: 360)
                    .padding(.horizontal)
            } else if vm.traffic.isEmpty {
                EmptyStateCard(title: "暂无流量数据", desc: "请调整时间范围后刷新")
                    .padding(.horizontal, UiSpec.pagePadding)
            } else {
                NpCard {
                    HStack(spacing: 14) {
                        Button {
                            showInSeries.toggle()
                        } label: {
                            Label("入方向", systemImage: showInSeries ? "circle.fill" : "circle")
                                .font(.caption)
                                .foregroundStyle(NpColor.indigo)
                        }
                        .buttonStyle(.plain)
                        Button {
                            showOutSeries.toggle()
                        } label: {
                            Label("出方向", systemImage: showOutSeries ? "circle.fill" : "circle")
                                .font(.caption)
                                .foregroundStyle(NpColor.success)
                        }
                        .buttonStyle(.plain)
                    }
                }
                .padding(.horizontal, UiSpec.pagePadding)
                ScrollView(.horizontal, showsIndicators: false) {
                    Chart {
                        ForEach(trafficForRender) { p in
                            if showInSeries {
                                LineMark(x: .value("时间", parseRFC3339(p.timestamp)), y: .value("入方向", p.traffic_in_bps ?? 0))
                                    .foregroundStyle(NpColor.indigo)
                                    .interpolationMethod(.catmullRom)
                                    .lineStyle(StrokeStyle(lineWidth: 1.9, lineCap: .round, lineJoin: .round))
                            }
                            if showOutSeries {
                                LineMark(x: .value("时间", parseRFC3339(p.timestamp)), y: .value("出方向", p.traffic_out_bps ?? 0))
                                    .foregroundStyle(NpColor.success)
                                    .interpolationMethod(.catmullRom)
                                    .lineStyle(StrokeStyle(lineWidth: 1.9, lineCap: .round, lineJoin: .round))
                            }
                        }
                    }
                    .transaction { $0.animation = nil }
                    .chartYAxis {
                        AxisMarks(position: .leading) { value in
                            AxisGridLine(stroke: StrokeStyle(lineWidth: 0.6, dash: [3, 4]))
                                .foregroundStyle(.white.opacity(0.14))
                            AxisTick().foregroundStyle(.white.opacity(0.45))
                            AxisValueLabel {
                                if let v = value.as(Double.self) {
                                    Text(formatBps(v)).foregroundStyle(.white.opacity(0.70))
                                } else if let v = value.as(Int.self) {
                                    Text(formatBps(Double(v))).foregroundStyle(.white.opacity(0.70))
                                }
                            }
                        }
                    }
                    .chartXAxis {
                        AxisMarks(values: .automatic(desiredCount: 5)) { _ in
                            AxisGridLine(stroke: StrokeStyle(lineWidth: 0.4, dash: [2, 5]))
                                .foregroundStyle(.white.opacity(0.08))
                            AxisTick().foregroundStyle(.white.opacity(0.4))
                            AxisValueLabel().foregroundStyle(.white.opacity(0.65))
                        }
                    }
                    .chartPlotStyle { plotArea in
                        plotArea
                            .background(Color(red: 21/255, green: 30/255, blue: 45/255))
                            .clipShape(RoundedRectangle(cornerRadius: 10))
                    }
                    .frame(
                        width: max(UIScreen.main.bounds.width - 32, CGFloat(trafficForRender.count) * 7.5),
                        height: 360
                    )
                }
                .padding(.horizontal)
            }
        }
        .background(NpColor.bg)
        .navigationTitle("端口详情")
        .navigationBarTitleDisplayMode(.inline)
        .overlay(alignment: .bottom) {
            if !vm.portError.isEmpty {
                Text(vm.portError)
                    .font(.footnote)
                    .foregroundStyle(NpColor.warning)
                    .padding(.bottom, 8)
            }
        }
        .task { await vm.fetchPortHistory(portID: portID, start: start, end: end) }
    }
}

func decimateTraffic(_ src: [InterfaceHistoryPoint], maxPoints: Int) -> [InterfaceHistoryPoint] {
    guard src.count > maxPoints, maxPoints > 0 else { return src }
    let bucket = Double(src.count) / Double(maxPoints)
    var out: [InterfaceHistoryPoint] = []
    out.reserveCapacity(maxPoints)
    var idx = 0.0
    while idx < Double(src.count) {
        let from = Int(idx)
        let to = min(src.count, Int(idx + bucket))
        guard from < to else { break }
        let slice = src[from..<to]
        let inAvg = slice.compactMap { $0.traffic_in_bps }.reduce(0, +) / Double(slice.count)
        let outAvg = slice.compactMap { $0.traffic_out_bps }.reduce(0, +) / Double(slice.count)
        let ts = slice.last?.timestamp ?? src[from].timestamp
        out.append(InterfaceHistoryPoint(timestamp: ts, traffic_in_bps: inAvg, traffic_out_bps: outAvg))
        idx += bucket
    }
    return out
}

func decimateDeviceHistory(_ src: [DeviceHistoryPoint], maxPoints: Int) -> [DeviceHistoryPoint] {
    guard src.count > maxPoints, maxPoints > 0 else { return src }
    let bucket = Double(src.count) / Double(maxPoints)
    var out: [DeviceHistoryPoint] = []
    out.reserveCapacity(maxPoints)
    var idx = 0.0
    while idx < Double(src.count) {
        let from = Int(idx)
        let to = min(src.count, Int(idx + bucket))
        guard from < to else { break }
        let slice = src[from..<to]
        let cpuAvg = slice.compactMap { $0.cpu_usage }.reduce(0, +) / Double(max(1, slice.compactMap { $0.cpu_usage }.count))
        let memAvg = slice.compactMap { $0.mem_usage }.reduce(0, +) / Double(max(1, slice.compactMap { $0.mem_usage }.count))
        let ts = slice.last?.timestamp ?? src[from].timestamp
        out.append(DeviceHistoryPoint(timestamp: ts, cpu_usage: cpuAvg, mem_usage: memAvg))
        idx += bucket
    }
    return out
}

func formatBps(_ value: Double) -> String {
    let absV = abs(value)
    if absV >= 1_000_000_000 { return String(format: "%.1fGbps", value / 1_000_000_000) }
    if absV >= 1_000_000 { return String(format: "%.1fMbps", value / 1_000_000) }
    if absV >= 1_000 { return String(format: "%.1fKbps", value / 1_000) }
    return String(format: "%.0fbps", value)
}

struct NpCard<Content: View>: View {
    @ViewBuilder var content: Content
    var body: some View {
        VStack(alignment: .leading, spacing: 8) { content }
            .padding(12)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(NpColor.card)
            .clipShape(RoundedRectangle(cornerRadius: UiSpec.cardRadius))
            .shadow(color: .black.opacity(0.08), radius: 8, x: 0, y: 2)
    }
}

struct CpuMemPanel: View {
    let historyLoading: Bool
    let cpuCurrent: Double
    let cpuPeak: Double
    let memCurrent: Double
    let memPeak: Double
    let cpuSeries: [UsageLinePoint]
    let memSeries: [UsageLinePoint]
    @State private var showCPU = true
    @State private var showMEM = true

    var body: some View {
        NpCard {
            Text("CPU / 内存").font(.headline.weight(.semibold))
            Text("CPU 当前 \(cpuCurrent, specifier: "%.1f")% / 峰值 \(cpuPeak, specifier: "%.1f")%")
                .font(.caption).foregroundStyle(.white.opacity(0.72))
            Text("内存 当前 \(memCurrent, specifier: "%.1f")% / 峰值 \(memPeak, specifier: "%.1f")%")
                .font(.caption).foregroundStyle(.white.opacity(0.72))
            HStack(spacing: 14) {
                Button { showCPU.toggle() } label: {
                    Label("CPU(%)", systemImage: showCPU ? "circle.fill" : "circle")
                        .font(.caption).foregroundStyle(Color.orange)
                }.buttonStyle(.plain)
                Button { showMEM.toggle() } label: {
                    Label("内存(%)", systemImage: showMEM ? "circle.fill" : "circle")
                        .font(.caption).foregroundStyle(Color.cyan)
                }.buttonStyle(.plain)
            }
            if historyLoading {
                ShimmerRect(height: 240)
            } else if cpuSeries.isEmpty && memSeries.isEmpty {
                EmptyStateCard(title: "暂无性能数据", desc: "等待下一轮采集后自动显示")
            } else {
                Chart {
                    RuleMark(y: .value("告警70", 70)).foregroundStyle(.yellow.opacity(0.55)).lineStyle(StrokeStyle(lineWidth: 1, dash: [5, 5]))
                    RuleMark(y: .value("告警85", 85)).foregroundStyle(.orange.opacity(0.55)).lineStyle(StrokeStyle(lineWidth: 1, dash: [5, 5]))
                    RuleMark(y: .value("告警90", 90)).foregroundStyle(.red.opacity(0.55)).lineStyle(StrokeStyle(lineWidth: 1, dash: [5, 5]))
                    if showCPU {
                        ForEach(cpuSeries) { p in
                            LineMark(x: .value("时间", p.ts), y: .value("CPU", p.value))
                                .foregroundStyle(Color.orange)
                        }
                    }
                    if showMEM {
                        ForEach(memSeries) { p in
                            LineMark(x: .value("时间", p.ts), y: .value("内存", p.value))
                                .foregroundStyle(Color.cyan)
                        }
                    }
                }
                .transaction { $0.animation = nil }
                .chartYScale(domain: 0...100)
                .chartYAxis {
                    AxisMarks(position: .leading, values: [0, 25, 50, 75, 100]) { value in
                        AxisGridLine(stroke: StrokeStyle(lineWidth: 0.5)).foregroundStyle(.white.opacity(0.10))
                        AxisTick().foregroundStyle(.white.opacity(0.35))
                        AxisValueLabel {
                            if let v = value.as(Double.self) {
                                Text("\(Int(v))%").foregroundStyle(.white.opacity(0.70))
                            }
                        }
                    }
                }
                .chartXAxis {
                    AxisMarks(values: .automatic(desiredCount: 4)) { _ in
                        AxisGridLine(stroke: StrokeStyle(lineWidth: 0.35)).foregroundStyle(.white.opacity(0.05))
                        AxisTick().foregroundStyle(.white.opacity(0.4))
                        AxisValueLabel().foregroundStyle(.white.opacity(0.65))
                    }
                }
                .chartPlotStyle { plotArea in
                    plotArea.background(Color(red: 21/255, green: 30/255, blue: 45/255)).clipShape(RoundedRectangle(cornerRadius: 10))
                }
                .frame(height: 260)
            }
        }
    }
}

struct ShimmerRect: View {
    let height: CGFloat
    @State private var phase: CGFloat = -220

    var body: some View {
        RoundedRectangle(cornerRadius: UiSpec.cardRadius)
            .fill(Color(red: 30/255, green: 41/255, blue: 59/255))
            .overlay(
                LinearGradient(
                    colors: [
                        Color(red: 30/255, green: 41/255, blue: 59/255),
                        Color(red: 15/255, green: 23/255, blue: 42/255),
                        Color(red: 30/255, green: 41/255, blue: 59/255)
                    ],
                    startPoint: .leading,
                    endPoint: .trailing
                )
                .rotationEffect(.degrees(8))
                .offset(x: phase)
                .blendMode(.plusLighter)
            )
            .clipped()
            .frame(height: height)
            .onAppear {
                withAnimation(.linear(duration: 1.5).repeatForever(autoreverses: false)) {
                    phase = 240
                }
            }
    }
}

struct PulseDot: View {
    let status: String
    @State private var scale: CGFloat = 0.85

    var body: some View {
        let color = status == "online" ? NpColor.success : (status == "offline" ? NpColor.danger : NpColor.warning)
        Circle()
            .fill(color)
            .frame(width: 10, height: 10)
            .overlay(Circle().stroke(color.opacity(0.45), lineWidth: 6).scaleEffect(scale).opacity(1.6 - scale))
            .onAppear {
                withAnimation(.easeOut(duration: 1.2).repeatForever(autoreverses: false)) {
                    scale = 1.8
                }
            }
    }
}

struct StatCard: View {
    let title: String
    let value: String
    let gradient: [Color]
    var pulse: Bool = false

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text(title).font(.caption).foregroundStyle(.white.opacity(0.9))
            HStack(spacing: 6) {
                if pulse { PulseDot(status: "online") }
                Text(value).font(.title3.bold()).foregroundStyle(.white)
            }
        }
        .padding(12)
        .frame(width: 150, alignment: .leading)
        .background(LinearGradient(colors: gradient, startPoint: .topLeading, endPoint: .bottomTrailing))
        .clipShape(RoundedRectangle(cornerRadius: UiSpec.cardRadius))
        .shadow(color: .black.opacity(0.1), radius: 6, x: 0, y: 2)
    }
}

extension Color {
    static let slate1 = Color(red: 51/255, green: 65/255, blue: 85/255)
    static let slate2 = Color(red: 30/255, green: 41/255, blue: 59/255)
    static let teal1 = Color(red: 15/255, green: 118/255, blue: 110/255)
    static let teal2 = Color(red: 6/255, green: 95/255, blue: 70/255)
    static let red1 = Color(red: 153/255, green: 27/255, blue: 27/255)
    static let red2 = Color(red: 127/255, green: 29/255, blue: 29/255)
    static let indigo1 = Color(red: 124/255, green: 58/255, blue: 237/255)
    static let indigo2 = Color(red: 79/255, green: 70/255, blue: 229/255)
}

enum Severity: String {
    case info, warning, error
}

func severity(of event: RecentEvent) -> Severity {
    let txt = ((event.level ?? "") + " " + (event.message ?? "")).uppercased()
    if txt.contains("OFFLINE") || txt.contains("ERROR") || txt.contains("RESTORE") { return .error }
    if txt.contains("WARN") || txt.contains("BGP") || txt.contains("OSPF") || txt.contains("FLAP") { return .warning }
    return .info
}

func severityColor(_ s: Severity) -> Color {
    switch s {
    case .error: return NpColor.danger
    case .warning: return NpColor.warning
    case .info: return NpColor.success
    }
}

func logLevelColor(_ level: String) -> Color {
    switch level.uppercased() {
    case "ERROR":
        return NpColor.danger
    case "WARNING", "WARN":
        return NpColor.warning
    default:
        return NpColor.success
    }
}

func statusText(_ status: String) -> String {
    switch status.lowercased() {
    case "online", "up": return "在线"
    case "offline", "down": return "离线"
    default: return "未知"
    }
}

func parseRFC3339(_ value: String) -> Date {
    let f = ISO8601DateFormatter()
    f.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
    if let d = f.date(from: value) { return d }
    let f0 = ISO8601DateFormatter()
    f0.formatOptions = [.withInternetDateTime]
    if let d = f0.date(from: value) { return d }
    let f2 = DateFormatter()
    f2.locale = Locale(identifier: "en_US_POSIX")
    f2.dateFormat = "yyyy-MM-dd'T'HH:mm:ssZ"
    if let d = f2.date(from: value) { return d }
    return .distantPast
}

func historyInterval(start: String, end: String) -> String {
    let fmt = ISO8601DateFormatter()
    guard let s = fmt.date(from: start), let e = fmt.date(from: end) else {
        return "1m"
    }
    let span = e.timeIntervalSince(s)
    if span > 180 * 24 * 3600 { return "1h" }
    if span > 30 * 24 * 3600 { return "5m" }
    return "1m"
}

struct EmptyStateCard: View {
    let title: String
    let desc: String

    var body: some View {
        VStack(spacing: 8) {
            Image(systemName: "tray")
                .font(.title2)
                .foregroundStyle(NpColor.indigo)
            Text(title).font(.headline)
            Text(desc).font(.footnote).foregroundStyle(.white.opacity(0.7))
        }
        .frame(maxWidth: .infinity)
        .padding(20)
        .background(NpColor.card)
        .clipShape(RoundedRectangle(cornerRadius: UiSpec.cardRadius))
        .shadow(color: .black.opacity(0.08), radius: 8, x: 0, y: 2)
    }
}

struct DeviceRow: View {
    let device: DeviceStatus
    var body: some View {
        NpCard {
            HStack(spacing: 8) {
                PulseDot(status: device.status)
                VStack(alignment: .leading, spacing: 4) {
                    Text(device.name.isEmpty ? device.ip : device.name)
                        .font(.headline)
                        .onLongPressGesture { UIPasteboard.general.string = device.ip }
                    Text("\(statusText(device.status)) · \(device.ip) · \(device.brand) · \(device.remark.isEmpty ? "未备注" : device.remark)")
                        .font(.subheadline)
                        .foregroundStyle(.white.opacity(0.7))
                    if let reason = device.status_reason, !reason.isEmpty {
                        Text(reason).font(.caption).foregroundStyle(.white.opacity(0.7))
                    }
                }
            }
        }
    }
}


struct DeviceQuickPeekSheet: View {
    @EnvironmentObject var vm: AppVM
    let device: DeviceStatus
    @State private var start = Calendar.current.date(byAdding: .day, value: -1, to: Date()) ?? Date()
    @State private var end = Date()

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(spacing: UiSpec.sectionGap) {
                    NpCard {
                        Text(device.name.isEmpty ? device.ip : device.name).font(.headline).foregroundStyle(.white)
                        Text("\(device.ip) · \(device.brand) · \(device.remark)").font(.footnote).foregroundStyle(.white.opacity(0.7))
                    }
                    NpCard {
                        Text("CPU / 内存").font(.headline.weight(.semibold)).foregroundStyle(.white)
                        HStack(spacing: 14) {
                            Label("CPU(%)", systemImage: "circle.fill").font(.caption).foregroundStyle(Color.orange)
                            Label("内存(%)", systemImage: "circle.fill").font(.caption).foregroundStyle(Color.cyan)
                        }
                        if vm.loading {
                            ShimmerRect(height: 220)
                        } else {
                            Chart {
                                ForEach(vm.cpu) { p in
                                    LineMark(x: .value("时间", parseRFC3339(p.timestamp)), y: .value("CPU", p.cpu_usage ?? 0)).foregroundStyle(Color.orange)
                                }
                                ForEach(vm.mem) { p in
                                    LineMark(x: .value("时间", parseRFC3339(p.timestamp)), y: .value("MEM", p.mem_usage ?? 0)).foregroundStyle(Color.cyan)
                                }
                            }
                            .chartYAxis {
                                AxisMarks(position: .leading) { _ in
                                    AxisGridLine(stroke: StrokeStyle(lineWidth: 0.6, dash: [3, 4]))
                                        .foregroundStyle(.white.opacity(0.14))
                                    AxisTick().foregroundStyle(.white.opacity(0.45))
                                    AxisValueLabel().foregroundStyle(.white.opacity(0.7))
                                }
                            }
                            .chartXAxis {
                                AxisMarks(values: .automatic(desiredCount: 4)) { _ in
                                    AxisGridLine(stroke: StrokeStyle(lineWidth: 0.4, dash: [2, 5]))
                                        .foregroundStyle(.white.opacity(0.08))
                                    AxisTick().foregroundStyle(.white.opacity(0.4))
                                    AxisValueLabel().foregroundStyle(.white.opacity(0.65))
                                }
                            }
                            .chartPlotStyle { plotArea in
                                plotArea
                                    .background(Color(red: 21/255, green: 30/255, blue: 45/255))
                                    .clipShape(RoundedRectangle(cornerRadius: 10))
                            }
                            .frame(height: 220)
                        }
                    }
                    NpCard {
                        Text("端口").font(.headline).foregroundStyle(.white)
                        ForEach(vm.deviceDetail?.interfaces ?? []) { p in
                            Text("\(p.name) (#\(p.index))").foregroundStyle(.white)
                        }
                    }
                }.padding(UiSpec.pagePadding)
            }
            .background(NpColor.bg)
            .navigationTitle("快速预览")
            .task {
                await vm.fetchDeviceDetail(deviceID: device.id)
                await vm.fetchDeviceHistory(deviceID: device.id, start: start, end: end)
            }
        }
    }
}
