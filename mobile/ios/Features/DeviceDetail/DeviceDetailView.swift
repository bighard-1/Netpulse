import SwiftUI

struct DeviceDetailView: View {
    let deviceID: String
    @StateObject var vm = DeviceDetailViewModel()
    @State private var showCPU = true
    @State private var showMem = true
    @State private var showPerformanceChart = false

    var body: some View {
        StatefulContainer(state: vm.state, retry: { vm.refresh(deviceID: deviceID) }) { d in
            List {
                Section("设备") {
                    Text(d.name.isEmpty ? d.ip : d.name).font(.headline)
                    Text("\(d.ip) · \(d.brand) · \(Fmt.status(d.status))")
                }

                Section("CPU/内存") {
                    Button {
                        withAnimation(.easeInOut(duration: 0.2)) {
                            showPerformanceChart.toggle()
                        }
                        if showPerformanceChart, vm.cpu.isEmpty, vm.mem.isEmpty {
                            vm.loadHistory(deviceID: deviceID)
                        }
                    } label: {
                        HStack {
                            VStack(alignment: .leading, spacing: 4) {
                                Text(showPerformanceChart ? "隐藏性能曲线" : "展开性能曲线")
                                    .font(.body.weight(.medium))
                                Text("默认隐藏，避免进入设备详情时加载过重")
                                    .font(.caption)
                                    .foregroundStyle(.secondary)
                            }
                            Spacer()
                            Image(systemName: showPerformanceChart ? "chevron.up.circle.fill" : "chevron.down.circle")
                                .foregroundStyle(Color.indigo)
                        }
                    }
                    .buttonStyle(.plain)

                    if showPerformanceChart {
                        HStack {
                            Toggle("CPU", isOn: $showCPU)
                            Toggle("内存", isOn: $showMem)
                        }
                        .toggleStyle(.switch)
                        .font(.subheadline)

                        if vm.loadingHistory {
                            ProgressView("加载监控数据...")
                        }

                        if vm.cpu.isEmpty && vm.mem.isEmpty {
                            Text("暂无监控数据")
                                .font(.caption)
                                .foregroundStyle(.secondary)
                        } else {
                            CpuMemChartView(cpu: vm.cpu, mem: vm.mem, showCPU: showCPU, showMem: showMem)
                                .frame(height: 220)
                        }
                    }
                }

                Section("端口") {
                    ForEach(d.interfaces) { p in
                        NavigationLink(value: AppRoute.port(deviceID: deviceID, portID: String(p.id))) {
                            HStack {
                                StatusDot(status: p.oper_status, adminStatus: p.admin_status)
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
        .navigationTitle("设备详情")
        .task { vm.load(deviceID: deviceID) }
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                Button("刷新") { vm.refresh(deviceID: deviceID) }
            }
        }
    }
}
