import SwiftUI
import LocalAuthentication

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

struct LoginResponse: Codable {
    struct U: Codable { let username: String; let role: String }
    let token: String
    let user: U
}

@MainActor
final class AppVM: ObservableObject {
    @Published var token: String = UserDefaults.standard.string(forKey: "token") ?? ""
    @Published var devices: [DeviceStatus] = []
    @Published var logs: [DeviceLog] = []
    @Published var loading = false
    @Published var err = ""

    var baseURL: String {
        get { UserDefaults.standard.string(forKey: "baseURL") ?? "http://119.40.55.18:18080/api" }
        set { UserDefaults.standard.set(newValue, forKey: "baseURL") }
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
            guard let h = resp as? HTTPURLResponse, (200..<300).contains(h.statusCode) else {
                throw NSError(domain: "login", code: 1)
            }
            let r = try JSONDecoder().decode(LoginResponse.self, from: data)
            token = r.token
            UserDefaults.standard.set(r.token, forKey: "token")
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
            if !u.isEmpty && !p.isEmpty {
                await login(u: u, p: p, remember: true)
            }
        }
    }

    func logout() {
        token = ""
        devices = []
        UserDefaults.standard.removeObject(forKey: "token")
    }

    func refreshDevices() async {
        guard !token.isEmpty else { return }
        loading = true
        defer { loading = false }
        do {
            var req = URLRequest(url: URL(string: "\(baseURL)/devices")!)
            req.addValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
            let (d, _) = try await URLSession.shared.data(for: req)
            devices = try JSONDecoder().decode([DeviceStatus].self, from: d)
        } catch {
            err = "加载设备失败"
        }
    }

    func fetchLogs(deviceID: Int64) async {
        do {
            var req = URLRequest(url: URL(string: "\(baseURL)/devices/\(deviceID)/logs")!)
            req.addValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
            let (d, _) = try await URLSession.shared.data(for: req)
            logs = try JSONDecoder().decode([DeviceLog].self, from: d)
        } catch {
            logs = []
        }
    }

    func updateInterfaceRemark(id: Int64, remark: String) async {
        do {
            var req = URLRequest(url: URL(string: "\(baseURL)/interfaces/\(id)/remark")!)
            req.httpMethod = "PUT"
            req.addValue("application/json", forHTTPHeaderField: "Content-Type")
            req.addValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
            req.httpBody = try JSONSerialization.data(withJSONObject: ["remark": remark])
            _ = try await URLSession.shared.data(for: req)
            await refreshDevices()
        } catch {}
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
                DeviceListView().environmentObject(vm)
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
        VStack(spacing: 10) {
            Text("NetPulse 移动端").font(.title2.bold())
            TextField("用户名", text: $u).textFieldStyle(.roundedBorder)
            SecureField("密码", text: $p).textFieldStyle(.roundedBorder)
            TextField("服务器 API 地址", text: $base).textFieldStyle(.roundedBorder)
            HStack {
                Button("登录") {
                    vm.baseURL = base
                    Task { await vm.login(u: u, p: p) }
                }
                .buttonStyle(.borderedProminent)
                Button("Face ID / Touch ID") {
                    Task { await vm.biometricLogin() }
                }
                .buttonStyle(.bordered)
            }
            Text("首次登录必须使用用户名密码").font(.footnote).foregroundStyle(.secondary)
            if !vm.err.isEmpty { Text(vm.err).foregroundStyle(.red) }
        }
        .padding()
    }
}

struct DeviceListView: View {
    @EnvironmentObject var vm: AppVM
    private var onlineCount: Int { vm.devices.filter { $0.status == "online" }.count }
    private var offlineCount: Int { vm.devices.count - onlineCount }

    var body: some View {
        NavigationStack {
            VStack(spacing: 10) {
                HStack {
                    Stat(title: "总数", value: "\(vm.devices.count)", color: .blue)
                    Stat(title: "在线", value: "\(onlineCount)", color: .green)
                    Stat(title: "离线", value: "\(offlineCount)", color: .red)
                }
                .padding(.horizontal)

                List(vm.devices) { d in
                    NavigationLink {
                        DeviceDetailView(device: d)
                            .environmentObject(vm)
                    } label: {
                        HStack(spacing: 10) {
                            Circle().fill(d.status == "online" ? .green : .red).frame(width: 10, height: 10)
                            VStack(alignment: .leading) {
                                Text(d.ip).font(.headline)
                                Text("\(d.brand) · \(d.remark)").font(.subheadline).foregroundStyle(.secondary)
                            }
                        }
                    }
                }
                .refreshable { await vm.refreshDevices() }
            }
            .navigationTitle("资产总览")
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button("退出") { vm.logout() }
                }
            }
            .task { await vm.refreshDevices() }
        }
    }
}

struct DeviceDetailView: View {
    @EnvironmentObject var vm: AppVM
    let device: DeviceStatus
    @State private var editIf: NetInterface?
    @State private var remarkText = ""

    var body: some View {
        List {
            Section("设备信息") {
                Text(device.ip)
                Text("\(device.brand) · \(device.remark)")
            }
            Section("端口列表（点击可编辑备注）") {
                ForEach(device.interfaces) { i in
                    Button {
                        editIf = i
                        remarkText = i.remark
                    } label: {
                        VStack(alignment: .leading) {
                            Text(i.name)
                            Text("备注: \(i.remark.isEmpty ? "-" : i.remark)")
                                .font(.footnote)
                                .foregroundStyle(.secondary)
                        }
                    }
                }
            }
            Section("最近日志") {
                ForEach(vm.logs) { l in
                    VStack(alignment: .leading, spacing: 4) {
                        Text(l.level).font(.caption.bold()).foregroundStyle(levelColor(l.level))
                        Text(l.message)
                        Text(l.created_at).font(.caption2).foregroundStyle(.secondary)
                    }
                }
            }
        }
        .navigationTitle("设备详情")
        .task { await vm.fetchLogs(deviceID: device.id) }
        .sheet(item: $editIf) { i in
            NavigationStack {
                Form {
                    Text(i.name)
                    TextField("备注", text: $remarkText)
                }
                .toolbar {
                    ToolbarItem(placement: .confirmationAction) {
                        Button("保存") {
                            Task {
                                await vm.updateInterfaceRemark(id: i.id, remark: remarkText)
                                editIf = nil
                            }
                        }
                    }
                }
            }
        }
    }

    private func levelColor(_ l: String) -> Color {
        switch l.uppercased() {
        case "ERROR": return .red
        case "WARNING": return .orange
        default: return .blue
        }
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
