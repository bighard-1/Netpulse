import SwiftUI

struct DeviceListView: View {
    @StateObject var vm = DeviceListViewModel()
    @State private var path = NavigationPath()
    @State private var quickSheetDeviceID: String = ""
    @State private var quickSheetPortID: String = ""
    @State private var quickSheetVisible = false

    var body: some View {
        NavigationStack(path: $path) {
            StatefulContainer(state: vm.state, retry: { Task { await vm.load() } }) { devices in
                List {
                    if !vm.results.isEmpty {
                        Section("搜索结果") {
                            ForEach(vm.results) { r in
                                Button {
                                    if r.category == .port, let pid = r.portID {
                                        quickSheetDeviceID = r.deviceID
                                        quickSheetPortID = pid
                                        quickSheetVisible = true
                                    } else {
                                        path.append(AppRoute.device(r.deviceID))
                                    }
                                } label: {
                                    VStack(alignment: .leading, spacing: 2) {
                                        HStack(spacing: 8) {
                                            Text(r.title).font(.body)
                                            if r.category == .port {
                                                Text("图表速览")
                                                    .font(.caption2)
                                                    .padding(.horizontal, 6)
                                                    .padding(.vertical, 2)
                                                    .background(Color.indigo.opacity(0.15))
                                                    .foregroundStyle(Color.indigo)
                                                    .clipShape(Capsule())
                                            }
                                        }
                                        Text(r.subtitle).font(.caption).foregroundStyle(.secondary)
                                    }
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
                .safeAreaInset(edge: .top) {
                    HStack(spacing: 8) {
                        Image(systemName: "magnifyingglass")
                            .foregroundStyle(.secondary)
                        TextField("搜索设备/IP/端口/备注", text: $vm.keyword)
                            .textInputAutocapitalization(.never)
                            .disableAutocorrection(true)
                            .onChange(of: vm.keyword) { _ in vm.onKeywordChanged(devices: devices) }
                        if vm.searching {
                            ProgressView().controlSize(.small)
                        }
                    }
                    .padding(.horizontal, 12)
                    .padding(.vertical, 10)
                    .background(.ultraThinMaterial)
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
            .sheet(isPresented: $quickSheetVisible) {
                NavigationStack {
                    PortDetailView(deviceID: quickSheetDeviceID, portID: quickSheetPortID)
                }
                .presentationDetents([.medium, .large])
                .presentationDragIndicator(.visible)
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
