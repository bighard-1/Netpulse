import SwiftUI
import Photos

struct PortDetailView: View {
    let deviceID: String
    let portID: String
    @StateObject var vm = PortDetailViewModel()
    @State private var showIn = true
    @State private var showOut = true
    @State private var toast = ""
    @State private var showToast = false

    var body: some View {
        VStack(spacing: 10) {
            Picker("范围", selection: $vm.preset) {
                Text("当日").tag(TimePreset.day)
                Text("近7天").tag(TimePreset.sevenDays)
                Text("近30天").tag(TimePreset.thirtyDays)
                Text("近1年").tag(TimePreset.oneYear)
                Text("近3年").tag(TimePreset.threeYears)
            }
            .pickerStyle(.segmented)
            .onChange(of: vm.preset) { _ in vm.load(portID: portID) }

            GroupBox("自定义时间段（3年内）") {
                VStack(spacing: 8) {
                    DatePicker("开始", selection: $vm.customStart, displayedComponents: [.date, .hourAndMinute])
                    DatePicker("结束", selection: $vm.customEnd, displayedComponents: [.date, .hourAndMinute])
                    HStack {
                        Button("取消") {
                            vm.customStart = Calendar.current.date(byAdding: .day, value: -1, to: Date()) ?? Date()
                            vm.customEnd = Date()
                        }
                        .buttonStyle(.bordered)
                        Spacer()
                        Button("查询") { vm.loadCustom(portID: portID) }
                            .buttonStyle(.borderedProminent)
                    }
                }
            }

            HStack {
                Toggle("入方向", isOn: $showIn)
                Toggle("出方向", isOn: $showOut)
                Spacer()
                Button("保存图表") { saveCurrentChart() }
                    .buttonStyle(.bordered)
            }
            .font(.subheadline)

            if !vm.loadingText.isEmpty {
                ProgressView(vm.loadingText)
            }

            StatefulContainer(state: vm.state, retry: { vm.load(portID: portID) }) { model in
                Group {
                    if model.decimated.isEmpty {
                        Text("该时间段暂无流量数据")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    } else {
                        TrafficChartView(model: model, showIn: showIn, showOut: showOut)
                            .frame(height: 320)
                    }
                }
            }
        }
        .padding(12)
        .navigationTitle("端口详情")
        .task { vm.load(portID: portID) }
        .alert("提示", isPresented: $showToast, actions: {
            Button("确定", role: .cancel) {}
        }, message: {
            Text(toast)
        })
    }

    private func presentToast(_ text: String) {
        toast = text
        showToast = true
    }

    private func saveCurrentChart() {
        guard case .success(let model) = vm.state else {
            presentToast("图表尚未就绪")
            return
        }
        let chart = TrafficChartExportView(model: model, showIn: showIn, showOut: showOut)
            .frame(width: 1280, height: 720)
            .padding(12)
            .background(.white)

        let renderer = ImageRenderer(content: chart)
        renderer.scale = 2
        renderer.isOpaque = true
        guard let image = renderer.uiImage else {
            presentToast("生成图片失败")
            return
        }

        PHPhotoLibrary.requestAuthorization(for: .addOnly) { status in
            guard status == .authorized || status == .limited else {
                DispatchQueue.main.async { presentToast("未授予相册权限") }
                return
            }
            UIImageWriteToSavedPhotosAlbum(image, nil, nil, nil)
            DispatchQueue.main.async { presentToast("已保存到相册") }
        }
    }
}
