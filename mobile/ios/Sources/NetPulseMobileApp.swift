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

struct DeviceStatus: Codable, Identifiable {
    let id: Int64
    let ip: String
    let brand: String
    let community: String?
    let remark: String
    let created_at: String
    let last_metric_at: String?
    let status: String
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
    @Published var deviceDetail: DeviceStatus?
    @Published var logs: [DeviceLog] = []
    @Published var cpu: [DeviceHistoryPoint] = []
    @Published var mem: [DeviceHistoryPoint] = []
    @Published var traffic: [InterfaceHistoryPoint] = []
    @Published var loading = false
    @Published var err = ""

    var baseURL: String {
        get { UserDefaults.standard.string(forKey: "baseURL") ?? "http://119.40.55.18:18080/api" }
        set { UserDefaults.standard.set(newValue, forKey: "baseURL") }
    }

    private func authorizedRequest(_ url: URL) -> URLRequest {
        var req = URLRequest(url: url)
        if !token.isEmpty {
            req.addValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }
        return req
    }

    private func dataWithAuth(_ req: URLRequest) async throws -> Data {
        let (data, resp) = try await URLSession.shared.data(for: req)
        if let http = resp as? HTTPURLResponse, http.statusCode == 401 {
            await MainActor.run {
                err = "登录已过期，请重新登录"
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
            guard let h = resp as? HTTPURLResponse, (200..<300).contains(h.statusCode) else { throw NSError(domain: "login", code: 1) }
            let r = try JSONDecoder().decode(LoginResponse.self, from: data)
            token = r.token
            KeychainStore.shared.set("netpulse_jwt", value: r.token)
            if remember {
                UserDefaults.standard.set(u, forKey: "u")
                UserDefaults.standard.set(p, forKey: "p")
            }
            await refreshDevices()
        } catch {
            err = "登录失败，请检查账号密码与地址"
        }
    }

    func biometricLogin() async {
        let ctx = LAContext()
        var e: NSError?
        guard ctx.canEvaluatePolicy(.deviceOwnerAuthenticationWithBiometrics, error: &e) else { return }
        if (try? await ctx.evaluatePolicy(.deviceOwnerAuthenticationWithBiometrics, localizedReason: "用于快速登录 NetPulse")) == true {
            let u = UserDefaults.standard.string(forKey: "u") ?? ""
            let p = UserDefaults.standard.string(forKey: "p") ?? ""
            if !u.isEmpty && !p.isEmpty { await login(u: u, p: p, remember: true) }
        }
    }

    func logout() {
        token = ""
        devices = []
        deviceDetail = nil
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
        } catch {
            err = "加载设备失败"
        }
    }

    func fetchDeviceDetail(deviceID: Int64) async {
        guard !token.isEmpty else { return }
        do {
            let req = authorizedRequest(URL(string: "\(baseURL)/devices/\(deviceID)")!)
            let d = try await dataWithAuth(req)
            deviceDetail = try JSONDecoder().decode(DeviceStatus.self, from: d)
        } catch {
            deviceDetail = nil
        }
    }

    func fetchDeviceHistory(deviceID: Int64, start: Date, end: Date) async {
        do {
            let s = ISO8601DateFormatter().string(from: start)
            let e = ISO8601DateFormatter().string(from: end)
            async let c = fetchHistory(type: "cpu", id: deviceID, start: s, end: e) as [DeviceHistoryPoint]
            async let m = fetchHistory(type: "mem", id: deviceID, start: s, end: e) as [DeviceHistoryPoint]
            async let l = fetchLogs(deviceID: deviceID)
            cpu = try await c
            mem = try await m
            logs = try await l
        } catch {
            cpu = []
            mem = []
            logs = []
        }
    }

    func fetchPortHistory(portID: Int64, start: Date, end: Date) async {
        do {
            let s = ISO8601DateFormatter().string(from: start)
            let e = ISO8601DateFormatter().string(from: end)
            traffic = try await fetchHistory(type: "traffic", id: portID, start: s, end: e)
        } catch {
            traffic = []
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
            err = "更新端口备注失败"
        }
    }

    private func fetchLogs(deviceID: Int64) async throws -> [DeviceLog] {
        let req = authorizedRequest(URL(string: "\(baseURL)/devices/\(deviceID)/logs")!)
        let d = try await dataWithAuth(req)
        return try JSONDecoder().decode([DeviceLog].self, from: d)
    }

    private func fetchHistory<T: Codable>(type: String, id: Int64, start: String, end: String) async throws -> [T] {
        var comp = URLComponents(string: "\(baseURL)/metrics/history")!
        comp.queryItems = [
            URLQueryItem(name: "type", value: type),
            URLQueryItem(name: "id", value: "\(id)"),
            URLQueryItem(name: "start", value: start),
            URLQueryItem(name: "end", value: end)
        ]
        let req = authorizedRequest(comp.url!)
        let d = try await dataWithAuth(req)
        return try JSONDecoder().decode(HistoryResp<T>.self, from: d).data
    }
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

    var body: some Scene {
        WindowGroup {
            if vm.token.isEmpty {
                LoginView().environmentObject(vm)
            } else {
                HomeView().environmentObject(vm)
            }
        }
    }
}

struct LoginView: View {
    @EnvironmentObject var vm: AppVM
    @State private var u = ""
    @State private var p = ""
    @State private var base = "http://119.40.55.18:18080/api"

    var body: some View {
        VStack(spacing: UiSpec.sectionGap) {
            Text("NetPulse 移动端").font(.title2.bold())
            TextField("用户名", text: $u).textFieldStyle(.roundedBorder)
            SecureField("密码", text: $p).textFieldStyle(.roundedBorder)
            TextField("服务器 API 地址", text: $base).textFieldStyle(.roundedBorder)
            HStack {
                Button("登录") {
                    vm.baseURL = base
                    Task { await vm.login(u: u, p: p) }
                }.buttonStyle(.borderedProminent)
                Button("生物识别快速登录") { Task { await vm.biometricLogin() } }.buttonStyle(.bordered)
            }
            Text("首次登录必须使用用户名密码").font(.footnote).foregroundStyle(.secondary)
            if !vm.err.isEmpty { Text(vm.err).foregroundStyle(.red) }
        }
        .padding(UiSpec.pagePadding)
    }
}

struct HomeView: View {
    @EnvironmentObject var vm: AppVM
    private var onlineCount: Int { vm.devices.filter { $0.status == "online" }.count }
    private var offlineCount: Int { vm.devices.filter { $0.status == "offline" }.count }
    private var unknownCount: Int { vm.devices.count - onlineCount - offlineCount }

    var body: some View {
        NavigationStack {
            VStack(spacing: UiSpec.sectionGap) {
                HStack {
                    Stat(title: "总数", value: "\(vm.devices.count)", color: .blue)
                    Stat(title: "在线", value: "\(onlineCount)", color: .green)
                    Stat(title: "离线", value: "\(offlineCount)", color: .red)
                    Stat(title: "未知", value: "\(unknownCount)", color: .orange)
                }
                .padding(.horizontal)

                List {
                    if vm.devices.isEmpty {
                        EmptyStateCard(title: "暂无资产", desc: "请先在 Web 端创建普通用户并添加设备")
                            .listRowSeparator(.hidden)
                    } else {
                        ForEach(vm.devices) { d in
                            NavigationLink(value: d.id) {
                                HStack(spacing: 10) {
                                    Circle().fill(statusColor(d.status)).frame(width: 10, height: 10)
                                    VStack(alignment: .leading) {
                                        Text(d.ip).font(.headline)
                                            .onLongPressGesture { UIPasteboard.general.string = d.ip }
                                        Text("\(d.brand) · \(d.remark.isEmpty ? "未备注" : d.remark)").font(.subheadline).foregroundStyle(.secondary)
                                    }
                                }
                            }
                        }
                    }
                }
                .listStyle(.insetGrouped)
                .refreshable { await vm.refreshDevices() }
            }
            .navigationTitle("资产总览")
            .navigationDestination(for: Int64.self) { id in
                DeviceDetailView(deviceID: id).environmentObject(vm)
            }
            .toolbar { ToolbarItem(placement: .topBarTrailing) { Button("退出") { vm.logout() } } }
            .task { await vm.refreshDevices() }
        }
    }
}

struct DeviceDetailView: View {
    @EnvironmentObject var vm: AppVM
    let deviceID: Int64
    @State private var keyword = ""
    @State private var editingPort: NetInterface?
    @State private var editingRemark = ""
    @State private var dateEnd = Date()
    @State private var dateStart = Calendar.current.date(byAdding: .day, value: -1, to: Date()) ?? Date()

    private var filteredPorts: [NetInterface] {
        let list = vm.deviceDetail?.interfaces ?? []
        let key = keyword.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        if key.isEmpty { return list }
        return list.filter {
            "\($0.id) \($0.index) \($0.name) \($0.remark)".lowercased().contains(key)
        }
    }

    var body: some View {
        List {
            Section("设备信息") {
                if let d = vm.deviceDetail {
                    Text(d.ip)
                        .onLongPressGesture { UIPasteboard.general.string = d.ip }
                    Text("\(d.brand) · \(d.remark)")
                }
            }

            Section("CPU / Memory") {
                Chart {
                    ForEach(vm.cpu) { p in
                        LineMark(x: .value("时间", p.timestamp), y: .value("CPU", p.cpu_usage ?? 0))
                            .foregroundStyle(.red)
                    }
                    ForEach(vm.mem) { p in
                        LineMark(x: .value("时间", p.timestamp), y: .value("内存", p.mem_usage ?? 0))
                            .foregroundStyle(.blue)
                    }
                }
                .frame(height: 260)
            }

            Section("端口搜索") {
                TextField("搜索端口 id/index/name/remark", text: $keyword)
                    .textFieldStyle(.roundedBorder)
            }

            Section("端口列表") {
                if filteredPorts.isEmpty {
                    EmptyStateCard(title: "暂无端口", desc: "SNMP 同步成功后会显示端口列表")
                }
                ForEach(filteredPorts) { p in
                    NavigationLink(value: p.id) {
                        VStack(alignment: .leading) {
                            Text(p.name)
                            Text("索引:\(p.index) · \(p.remark.isEmpty ? "-" : p.remark)")
                                .font(.footnote)
                                .foregroundStyle(.secondary)
                            Text("点击看流量，长按改备注")
                                .font(.caption2)
                                .foregroundStyle(.secondary)
                        }
                    }
                    .contextMenu {
                        Button("编辑端口备注") {
                            editingPort = p
                            editingRemark = p.remark
                        }
                    }
                }
            }

            Section("最近日志") {
                if vm.logs.isEmpty {
                    Text("暂无日志").foregroundStyle(.secondary)
                } else {
                    ForEach(vm.logs.prefix(100)) { log in
                        VStack(alignment: .leading, spacing: 4) {
                            Text("[\(log.level)]")
                                .font(.caption.bold())
                                .foregroundStyle(logLevelColor(log.level))
                            Text(log.message).font(.footnote)
                            Text(log.created_at).font(.caption2).foregroundStyle(.secondary)
                        }
                    }
                }
            }
        }
        .navigationTitle("设备详情")
        .navigationBarTitleDisplayMode(.inline)
        .navigationDestination(for: Int64.self) { portID in
            PortDetailView(deviceID: deviceID, portID: portID)
                .environmentObject(vm)
        }
        .toolbar { ToolbarItem(placement: .topBarLeading) { Text("返回设备").font(.footnote).foregroundStyle(.secondary) } }
        .task {
            await vm.fetchDeviceDetail(deviceID: deviceID)
            dateEnd = Date()
            dateStart = Calendar.current.date(byAdding: .day, value: -1, to: dateEnd) ?? dateEnd
            await vm.fetchDeviceHistory(deviceID: deviceID, start: dateStart, end: dateEnd)
        }
        .refreshable {
            await vm.fetchDeviceDetail(deviceID: deviceID)
            dateEnd = Date()
            dateStart = Calendar.current.date(byAdding: .day, value: -1, to: dateEnd) ?? dateEnd
            await vm.fetchDeviceHistory(deviceID: deviceID, start: dateStart, end: dateEnd)
        }
        .sheet(item: $editingPort) { port in
            NavigationStack {
                Form {
                    Section("端口") { Text(port.name) }
                    Section("备注") { TextField("请输入备注", text: $editingRemark) }
                }
                .navigationTitle("编辑端口备注")
                .toolbar {
                    ToolbarItem(placement: .cancellationAction) {
                        Button("取消") { editingPort = nil }
                    }
                    ToolbarItem(placement: .confirmationAction) {
                        Button("保存") {
                            Task {
                                await vm.updateInterfaceRemark(
                                    interfaceID: port.id,
                                    remark: editingRemark.trimmingCharacters(in: .whitespacesAndNewlines),
                                    deviceID: deviceID,
                                    start: dateStart,
                                    end: dateEnd
                                )
                                editingPort = nil
                            }
                        }
                    }
                }
            }
        }
    }
}

struct PortDetailView: View {
    @EnvironmentObject var vm: AppVM
    let deviceID: Int64
    let portID: Int64
    @State private var start = Calendar.current.date(byAdding: .day, value: -1, to: Date()) ?? Date()
    @State private var end = Date()

    private var minDate: Date {
        Calendar.current.date(byAdding: .year, value: -3, to: Date()) ?? .distantPast
    }

    var body: some View {
        VStack(spacing: UiSpec.sectionGap) {
            HStack {
                DatePicker("开始", selection: $start, in: minDate...Date(), displayedComponents: [.date, .hourAndMinute])
                DatePicker("结束", selection: $end, in: minDate...Date(), displayedComponents: [.date, .hourAndMinute])
            }
            .padding(.horizontal)

            Button("刷新流量") {
                Task { await vm.fetchPortHistory(portID: portID, start: start, end: end) }
            }
            .buttonStyle(.borderedProminent)

            if vm.traffic.isEmpty {
                EmptyStateCard(title: "暂无流量数据", desc: "请调整时间范围后刷新")
                    .padding(.horizontal, UiSpec.pagePadding)
            } else {
                Chart {
                    ForEach(vm.traffic) { p in
                        LineMark(x: .value("时间", p.timestamp), y: .value("入方向", p.traffic_in_bps ?? 0))
                            .foregroundStyle(.green)
                        LineMark(x: .value("时间", p.timestamp), y: .value("出方向", p.traffic_out_bps ?? 0))
                            .foregroundStyle(.orange)
                    }
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity)
                .padding(.horizontal)
            }
        }
        .navigationTitle("端口流量")
        .navigationBarTitleDisplayMode(.inline)
        .task { await vm.fetchPortHistory(portID: portID, start: start, end: end) }
    }
}

struct Stat: View {
    let title: String
    let value: String
    let color: Color
    var body: some View {
        VStack {
            Text(value).font(.title3.bold()).foregroundStyle(color)
            Text(title).font(.caption).foregroundStyle(.secondary)
        }
        .frame(maxWidth: .infinity)
        .padding(10)
        .background(.thinMaterial)
        .clipShape(RoundedRectangle(cornerRadius: 10))
    }
}

struct EmptyStateCard: View {
    let title: String
    let desc: String

    var body: some View {
        VStack(spacing: 8) {
            Image(systemName: "tray")
                .font(.title2)
                .foregroundStyle(.blue)
            Text(title).font(.headline)
            Text(desc).font(.footnote).foregroundStyle(.secondary)
        }
        .frame(maxWidth: .infinity)
        .padding(20)
        .background(Color(.systemGray6))
        .clipShape(RoundedRectangle(cornerRadius: UiSpec.cardRadius))
    }
}

func logLevelColor(_ level: String) -> Color {
    switch level.uppercased() {
    case "ERROR":
        return .red
    case "WARNING", "WARN":
        return .orange
    default:
        return .green
    }
}

func statusColor(_ status: String) -> Color {
    switch status {
    case "online":
        return .green
    case "offline":
        return .red
    default:
        return .orange
    }
}
