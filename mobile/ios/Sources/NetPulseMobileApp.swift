import SwiftUI
import Charts
import LocalAuthentication
import Security

// MARK: - Models

struct LoginRequest: Codable {
    let username: String
    let password: String
}

struct LoginResponse: Codable {
    struct LoginUser: Codable {
        let username: String
        let role: String
    }
    let token: String
    let user: LoginUser
}

struct DeviceStatus: Codable, Identifiable {
    let id: Int64
    let ip: String
    let name: String
    let brand: String
    let remark: String
    let status: String
    let interfaces: [NetInterface]
}

struct NetInterface: Codable, Identifiable {
    let id: Int64
    let index: Int
    let name: String
    let remark: String
    let custom_name: String?
    let oper_status: Int?
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

struct HistoryResponse<T: Codable>: Codable {
    let type: String
    let id: Int64
    let data: [T]
}

// MARK: - Keychain

final class KeychainStore {
    static let shared = KeychainStore()

    func set(_ value: String, key: String) {
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
        var result: AnyObject?
        guard SecItemCopyMatching(query as CFDictionary, &result) == errSecSuccess,
              let data = result as? Data else {
            return nil
        }
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

// MARK: - API

struct NetPulseAPI {
    var baseURL: String
    var token: String

    enum APIError: Error {
        case invalidURL
        case http(Int, String)
        case decode(String)
    }

    private func request(path: String, method: String = "GET", body: Data? = nil) throws -> URLRequest {
        guard let url = URL(string: "\(baseURL)\(path)") else { throw APIError.invalidURL }
        var req = URLRequest(url: url)
        req.httpMethod = method
        req.timeoutInterval = 20
        if !token.isEmpty {
            req.addValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }
        if body != nil {
            req.addValue("application/json", forHTTPHeaderField: "Content-Type")
        }
        req.httpBody = body
        return req
    }

    private func send(_ req: URLRequest) async throws -> Data {
        let (data, resp) = try await URLSession.shared.data(for: req)
        let code = (resp as? HTTPURLResponse)?.statusCode ?? 500
        guard 200 ..< 300 ~= code else {
            let text = String(data: data, encoding: .utf8) ?? "HTTP \(code)"
            throw APIError.http(code, text)
        }
        return data
    }

    func login(username: String, password: String) async throws -> LoginResponse {
        let body = try JSONEncoder().encode(LoginRequest(username: username, password: password))
        let req = try request(path: "/login", method: "POST", body: body)
        let data = try await send(req)
        return try JSONDecoder().decode(LoginResponse.self, from: data)
    }

    func devices() async throws -> [DeviceStatus] {
        let req = try request(path: "/devices")
        let data = try await send(req)
        return try JSONDecoder().decode([DeviceStatus].self, from: data)
    }

    func device(id: Int64) async throws -> DeviceStatus {
        let req = try request(path: "/devices/\(id)")
        let data = try await send(req)
        return try JSONDecoder().decode(DeviceStatus.self, from: data)
    }

    func history<T: Codable>(type: String, id: Int64, start: Date, end: Date, maxPoints: Int, interval: String) async throws -> [T] {
        let iso = ISO8601DateFormatter()
        let s = iso.string(from: start)
        let e = iso.string(from: end)
        let path = "/metrics/history?type=\(type)&id=\(id)&start=\(s)&end=\(e)&max_points=\(maxPoints)&interval=\(interval)"
        let req = try request(path: path)
        let data = try await send(req)
        return try JSONDecoder().decode(HistoryResponse<T>.self, from: data).data
    }
}

// MARK: - App State

@MainActor
final class AppState: ObservableObject {
    @Published var token: String = KeychainStore.shared.get("np_jwt") ?? ""
    @Published var username: String = UserDefaults.standard.string(forKey: "np_user") ?? ""
    @Published var baseURL: String = UserDefaults.standard.string(forKey: "np_base") ?? "http://119.40.55.18:18080/api"
    @Published var loginError: String = ""

    @Published var devices: [DeviceStatus] = []
    @Published var listLoading = false

    private var api: NetPulseAPI { NetPulseAPI(baseURL: baseURL, token: token) }

    func login(user: String, pass: String) async {
        loginError = ""
        do {
            let res = try await NetPulseAPI(baseURL: baseURL, token: "").login(username: user, password: pass)
            token = res.token
            username = res.user.username
            KeychainStore.shared.set(res.token, key: "np_jwt")
            UserDefaults.standard.set(baseURL, forKey: "np_base")
            UserDefaults.standard.set(username, forKey: "np_user")
            await refreshDevices()
        } catch {
            loginError = friendly(error)
        }
    }

    func biometricLogin() async {
        let ctx = LAContext()
        var err: NSError?
        guard ctx.canEvaluatePolicy(.deviceOwnerAuthenticationWithBiometrics, error: &err) else {
            loginError = "设备不支持生物识别"
            return
        }
        do {
            let ok = try await ctx.evaluatePolicy(.deviceOwnerAuthenticationWithBiometrics, localizedReason: "登录 NetPulse")
            if ok, !token.isEmpty {
                await refreshDevices()
            }
        } catch {
            loginError = "生物识别失败"
        }
    }

    func logout() {
        token = ""
        devices = []
        KeychainStore.shared.delete("np_jwt")
    }

    func refreshDevices() async {
        guard !token.isEmpty else { return }
        listLoading = true
        defer { listLoading = false }
        do {
            devices = try await api.devices()
        } catch {
            loginError = friendly(error)
        }
    }

    func friendly(_ e: Error) -> String {
        if let apiErr = e as? NetPulseAPI.APIError {
            switch apiErr {
            case .http(let c, let m): return "请求失败(\(c)): \(m)"
            case .decode(let m): return "数据解析失败: \(m)"
            case .invalidURL: return "地址无效"
            }
        }
        return e.localizedDescription
    }
}

@MainActor
final class DeviceDetailVM: ObservableObject {
    enum State {
        case loading
        case success(DeviceStatus)
        case error(String)
    }

    @Published var state: State = .loading
    @Published var cpuSeries: [DeviceHistoryPoint] = []
    @Published var memSeries: [DeviceHistoryPoint] = []
    @Published var historyLoading = false

    private var seq = 0
    private let deviceID: Int64

    init(deviceID: Int64) {
        self.deviceID = deviceID
    }

    func load(baseURL: String, token: String) async {
        guard !token.isEmpty else { return }
        seq += 1
        let cur = seq
        state = .loading
        let api = NetPulseAPI(baseURL: baseURL, token: token)
        do {
            let d = try await api.device(id: deviceID)
            guard cur == seq else { return }
            state = .success(d)
            await loadHistory(baseURL: baseURL, token: token)
        } catch {
            guard cur == seq else { return }
            state = .error((error as NSError).localizedDescription)
        }
    }

    func loadHistory(baseURL: String, token: String) async {
        guard !token.isEmpty else { return }
        historyLoading = true
        defer { historyLoading = false }
        let api = NetPulseAPI(baseURL: baseURL, token: token)
        let end = Date()
        let start = Calendar.current.date(byAdding: .day, value: -1, to: end) ?? end
        do {
            async let cpu: [DeviceHistoryPoint] = api.history(type: "cpu", id: deviceID, start: start, end: end, maxPoints: 1440, interval: "1m")
            async let mem: [DeviceHistoryPoint] = api.history(type: "mem", id: deviceID, start: start, end: end, maxPoints: 1440, interval: "1m")
            cpuSeries = try await cpu
            memSeries = try await mem
        } catch {
            cpuSeries = []
            memSeries = []
        }
    }
}

@MainActor
final class PortDetailVM: ObservableObject {
    enum State {
        case loading
        case success
        case error(String)
    }

    @Published var state: State = .loading
    @Published var points: [InterfaceHistoryPoint] = []

    private var seq = 0
    let portID: Int64

    init(portID: Int64) { self.portID = portID }

    func load(baseURL: String, token: String, start: Date, end: Date, interval: String, maxPoints: Int) async {
        guard !token.isEmpty else { return }
        seq += 1
        let cur = seq
        state = .loading
        let api = NetPulseAPI(baseURL: baseURL, token: token)
        do {
            let data: [InterfaceHistoryPoint] = try await api.history(type: "traffic", id: portID, start: start, end: end, maxPoints: maxPoints, interval: interval)
            guard cur == seq else { return }
            points = data.sorted { $0.timestamp < $1.timestamp }
            state = .success
        } catch {
            guard cur == seq else { return }
            points = []
            state = .error((error as NSError).localizedDescription)
        }
    }
}

// MARK: - Views

@main
struct NetPulseMobileApp: App {
    @StateObject private var app = AppState()

    var body: some Scene {
        WindowGroup {
            if app.token.isEmpty {
                LoginView().environmentObject(app)
            } else {
                DeviceListView().environmentObject(app)
            }
        }
    }
}

struct LoginView: View {
    @EnvironmentObject var app: AppState
    @State private var user = ""
    @State private var pass = ""

    var body: some View {
        VStack(spacing: 12) {
            Text("NetPulse").font(.largeTitle).bold()
            TextField("用户名", text: $user).textFieldStyle(.roundedBorder)
            SecureField("密码", text: $pass).textFieldStyle(.roundedBorder)
            TextField("API 地址", text: $app.baseURL).textFieldStyle(.roundedBorder)
            Button("登录") { Task { await app.login(user: user, pass: pass) } }
                .buttonStyle(.borderedProminent)
            Button("Face ID / Touch ID") { Task { await app.biometricLogin() } }
                .buttonStyle(.bordered)
            if !app.loginError.isEmpty {
                Text(app.loginError).foregroundStyle(.red).font(.footnote)
            }
        }
        .padding(24)
    }
}

struct DeviceListView: View {
    @EnvironmentObject var app: AppState
    @State private var keyword = ""

    private var portMatches: [(DeviceStatus, NetInterface)] {
        let k = keyword.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        guard !k.isEmpty else { return [] }
        return app.devices.flatMap { d in
            d.interfaces.compactMap { p in
                let nm = (p.custom_name?.isEmpty == false ? p.custom_name! : p.name)
                let blob = "\(d.name) \(d.ip) \(nm) \(p.remark) \(p.index)".lowercased()
                return blob.contains(k) ? (d, p) : nil
            }
        }.prefix(30).map { $0 }
    }

    var body: some View {
        NavigationStack {
            List {
                Section("搜索") {
                    TextField("设备/IP/端口", text: $keyword)
                    if !portMatches.isEmpty {
                        ForEach(Array(portMatches.enumerated()), id: \ .offset) { _, pair in
                            let (d, p) = pair
                            let title = (p.custom_name?.isEmpty == false ? p.custom_name! : p.name)
                            NavigationLink {
                                PortDetailView(deviceID: d.id, portID: p.id)
                                    .environmentObject(app)
                            } label: {
                                HStack {
                                    PortStatusDot(status: p.oper_status)
                                    Text("\(title) · \(d.name.isEmpty ? d.ip : d.name)")
                                }
                            }
                        }
                    }
                }
                Section("设备") {
                    if app.listLoading {
                        ProgressView("加载中")
                    } else {
                        ForEach(app.devices) { d in
                            NavigationLink {
                                DeviceDetailView(deviceID: d.id).environmentObject(app)
                            } label: {
                                VStack(alignment: .leading, spacing: 4) {
                                    Text(d.name.isEmpty ? d.ip : d.name).font(.headline)
                                    Text("\(statusText(d.status)) · \(d.ip) · \(d.brand) · \(d.remark)")
                                        .font(.caption)
                                        .foregroundStyle(.secondary)
                                }
                            }
                        }
                    }
                }
            }
            .navigationTitle("资产中心")
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button("退出") { app.logout() }
                }
                ToolbarItem(placement: .topBarTrailing) {
                    Button("刷新") { Task { await app.refreshDevices() } }
                }
            }
            .task { await app.refreshDevices() }
        }
    }
}

struct DeviceDetailView: View {
    @EnvironmentObject var app: AppState
    let deviceID: Int64
    @StateObject private var vm: DeviceDetailVM

    init(deviceID: Int64) {
        self.deviceID = deviceID
        _vm = StateObject(wrappedValue: DeviceDetailVM(deviceID: deviceID))
    }

    var body: some View {
        Group {
            switch vm.state {
            case .loading:
                ProgressView("加载设备详情...")
            case .error(let msg):
                VStack(spacing: 12) {
                    Text("加载失败").font(.headline)
                    Text(msg).font(.footnote).foregroundStyle(.red)
                    Button("重试") { Task { await vm.load(baseURL: app.baseURL, token: app.token) } }
                }
            case .success(let d):
                List {
                    Section("设备") {
                        Text(d.name.isEmpty ? d.ip : d.name).font(.headline)
                        Text("\(statusText(d.status)) · \(d.ip) · \(d.brand)")
                    }
                    Section("CPU / 内存") {
                        CpuMemChart(cpu: vm.cpuSeries, mem: vm.memSeries, loading: vm.historyLoading)
                            .frame(height: 260)
                    }
                    Section("端口") {
                        ForEach(d.interfaces) { p in
                            NavigationLink {
                                PortDetailView(deviceID: d.id, portID: p.id).environmentObject(app)
                            } label: {
                                HStack {
                                    PortStatusDot(status: p.oper_status)
                                    VStack(alignment: .leading) {
                                        Text((p.custom_name?.isEmpty == false ? p.custom_name! : p.name))
                                        Text("索引:\(p.index) · \(p.remark)")
                                            .font(.caption)
                                            .foregroundStyle(.secondary)
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }
        .navigationTitle("设备详情")
        .navigationBarTitleDisplayMode(.inline)
        .task { await vm.load(baseURL: app.baseURL, token: app.token) }
    }
}

struct PortDetailView: View {
    @EnvironmentObject var app: AppState
    let deviceID: Int64
    let portID: Int64

    @StateObject private var vm: PortDetailVM
    @State private var showIn = true
    @State private var showOut = true
    @State private var range: Int = 0 // 0 day,1 7d,2 30d

    init(deviceID: Int64, portID: Int64) {
        self.deviceID = deviceID
        self.portID = portID
        _vm = StateObject(wrappedValue: PortDetailVM(portID: portID))
    }

    var body: some View {
        VStack(spacing: 12) {
            Picker("范围", selection: $range) {
                Text("当日").tag(0)
                Text("近7天").tag(1)
                Text("近30天").tag(2)
            }
            .pickerStyle(.segmented)
            .onChange(of: range) { _ in Task { await reload() } }

            HStack(spacing: 12) {
                Button(showIn ? "隐藏入方向" : "显示入方向") { showIn.toggle() }
                    .buttonStyle(.bordered)
                Button(showOut ? "隐藏出方向" : "显示出方向") { showOut.toggle() }
                    .buttonStyle(.bordered)
            }

            switch vm.state {
            case .loading:
                ProgressView("加载流量中...")
            case .error(let msg):
                VStack(spacing: 8) {
                    Text("加载失败").font(.headline)
                    Text(msg).font(.footnote).foregroundStyle(.red)
                    Button("重试") { Task { await reload() } }
                }
            case .success:
                if vm.points.isEmpty {
                    Text("暂无流量数据").foregroundStyle(.secondary)
                } else {
                    TrafficChart(points: vm.points, showIn: showIn, showOut: showOut)
                        .frame(height: 320)
                }
            }
            Spacer()
        }
        .padding(16)
        .navigationTitle("端口详情")
        .navigationBarTitleDisplayMode(.inline)
        .task { await reload() }
    }

    private func reload() async {
        let end = Date()
        let start: Date
        let interval: String
        let maxPoints: Int
        switch range {
        case 1:
            start = Calendar.current.date(byAdding: .day, value: -7, to: end) ?? end
            interval = "5m"
            maxPoints = 2400
        case 2:
            start = Calendar.current.date(byAdding: .day, value: -30, to: end) ?? end
            interval = "1h"
            maxPoints = 1500
        default:
            start = Calendar.current.startOfDay(for: end)
            interval = "1m"
            maxPoints = 1800
        }
        await vm.load(baseURL: app.baseURL, token: app.token, start: start, end: end, interval: interval, maxPoints: maxPoints)
    }
}

// MARK: - Charts

struct CpuMemChart: View {
    let cpu: [DeviceHistoryPoint]
    let mem: [DeviceHistoryPoint]
    let loading: Bool

    var body: some View {
        if loading {
            ProgressView("加载图表...")
        } else {
            Chart {
                RuleMark(y: .value("70", 70)).foregroundStyle(.yellow.opacity(0.45)).lineStyle(StrokeStyle(lineWidth: 1, dash: [5,5]))
                RuleMark(y: .value("85", 85)).foregroundStyle(.orange.opacity(0.45)).lineStyle(StrokeStyle(lineWidth: 1, dash: [5,5]))
                RuleMark(y: .value("90", 90)).foregroundStyle(.red.opacity(0.45)).lineStyle(StrokeStyle(lineWidth: 1, dash: [5,5]))

                ForEach(cpu) { p in
                    if let v = finite(p.cpu_usage) {
                        LineMark(x: .value("时间", parseTs(p.timestamp)), y: .value("CPU", v))
                            .foregroundStyle(.orange)
                            .lineStyle(StrokeStyle(lineWidth: 2, dash: [8,4]))
                    }
                }
                ForEach(mem) { p in
                    if let v = finite(p.mem_usage) {
                        LineMark(x: .value("时间", parseTs(p.timestamp)), y: .value("内存", v))
                            .foregroundStyle(.cyan)
                            .lineStyle(StrokeStyle(lineWidth: 2))
                    }
                }
            }
            .chartYScale(domain: 0...100) // fixed domain
            .chartYAxis {
                AxisMarks(position: .leading, values: [0, 25, 50, 75, 100]) { value in
                    AxisGridLine()
                    AxisTick()
                    AxisValueLabel {
                        if let v = value.as(Double.self) { Text("\(Int(v))%") }
                    }
                }
            }
        }
    }
}

struct TrafficChart: View {
    let points: [InterfaceHistoryPoint]
    let showIn: Bool
    let showOut: Bool

    private var globalMax: Double {
        let vals = points.flatMap { [finite($0.traffic_in_bps), finite($0.traffic_out_bps)] }.compactMap { $0 }
        return max(1, (vals.max() ?? 1) * 1.1)
    }

    var body: some View {
        Chart {
            ForEach(points) { p in
                if showIn, let v = finite(p.traffic_in_bps) {
                    LineMark(x: .value("时间", parseTs(p.timestamp)), y: .value("入", v))
                        .foregroundStyle(Color(red: 99/255, green: 102/255, blue: 241/255))
                        .lineStyle(StrokeStyle(lineWidth: 2, dash: [8,4]))
                }
                if showOut, let v = finite(p.traffic_out_bps) {
                    LineMark(x: .value("时间", parseTs(p.timestamp)), y: .value("出", v))
                        .foregroundStyle(Color(red: 34/255, green: 197/255, blue: 94/255))
                        .lineStyle(StrokeStyle(lineWidth: 2))
                }
            }
        }
        .chartYScale(domain: 0...globalMax) // locked by full dataset max
        .chartYAxis {
            AxisMarks(position: .leading) { value in
                AxisGridLine()
                AxisTick()
                AxisValueLabel {
                    if let v = value.as(Double.self) { Text(formatBps(v)) }
                }
            }
        }
        .chartLegend(position: .top)
    }
}

// MARK: - Helpers

func finite(_ v: Double?) -> Double? {
    guard let x = v, x.isFinite else { return nil }
    return x
}

func parseTs(_ str: String) -> Date {
    let f = ISO8601DateFormatter()
    f.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
    if let d = f.date(from: str) { return d }
    let f2 = ISO8601DateFormatter()
    f2.formatOptions = [.withInternetDateTime]
    if let d = f2.date(from: str) { return d }
    return .distantPast
}

func formatBps(_ value: Double) -> String {
    let a = abs(value)
    if a >= 1_000_000_000 { return String(format: "%.1fGbps", value / 1_000_000_000) }
    if a >= 1_000_000 { return String(format: "%.1fMbps", value / 1_000_000) }
    if a >= 1_000 { return String(format: "%.1fKbps", value / 1_000) }
    return String(format: "%.0fbps", value)
}

func statusText(_ s: String) -> String {
    switch s.lowercased() {
    case "online", "up": return "在线"
    case "offline", "down": return "离线"
    default: return "未知"
    }
}

struct PortStatusDot: View {
    let status: Int?
    var color: Color {
        switch status {
        case 1: return .green
        case 2: return .red
        default: return .yellow
        }
    }

    var body: some View {
        Circle().fill(color).frame(width: 10, height: 10)
    }
}
