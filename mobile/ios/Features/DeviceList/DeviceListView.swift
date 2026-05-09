import SwiftUI

struct DeviceListView: View {
    @StateObject var vm = DeviceListViewModel()
    @State private var path = NavigationPath()

    var body: some View {
        NavigationStack(path: $path) {
            StatefulContainer(state: vm.state, retry: { Task { await vm.load() } }) { devices in
                List {
                    Section("全局搜索") {
                        HStack {
                            TextField("设备/IP/端口/备注", text: $vm.keyword)
                                .onChange(of: vm.keyword) { _ in vm.onKeywordChanged(devices: devices) }
                            if vm.searching {
                                ProgressView().controlSize(.small)
                            }
                        }
                        ForEach(vm.results) { r in
                            Button {
                                if r.category == .port, let pid = r.portID {
                                    path.append(AppRoute.port(deviceID: r.deviceID, portID: pid))
                                } else {
                                    path.append(AppRoute.device(r.deviceID))
                                }
                            } label: {
                                VStack(alignment: .leading, spacing: 2) {
                                    Text(r.title).font(.body)
                                    Text(r.subtitle).font(.caption).foregroundStyle(.secondary)
                                }
                            }
                        }
                    }

                    Section("资产") {
                        ForEach(devices) { d in
                            Button {
                                path.append(AppRoute.device(String(d.id)))
                            } label: {
                                HStack {
                                    StatusDot(status: d.status)
                                    VStack(alignment: .leading) {
                                        Text(d.name.isEmpty ? d.ip : d.name)
                                        Text("\(d.ip) · \(d.brand) · \(Fmt.status(d.status))")
                                            .font(.caption)
                                            .foregroundStyle(.secondary)
                                    }
                                }
                            }
                        }
                    }
                }
            }
            .navigationTitle("资产中心")
            .navigationDestination(for: AppRoute.self) { route in
                switch route {
                case .device(let id):
                    DeviceDetailView(deviceID: id)
                case .port(let did, let pid):
                    PortDetailView(deviceID: did, portID: pid)
                }
            }
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button("刷新") { Task { await vm.load() } }
                }
                ToolbarItem(placement: .topBarLeading) {
                    Button("退出") {
                        KeychainManager.clearToken()
                        AuthGate.shared.setAuthenticated(false)
                    }
                }
            }
            .task { await vm.load() }
        }
    }
}
