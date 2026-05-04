import SwiftUI

struct Device: Identifiable {
    let id: Int
    let ip: String
    let brand: String
    let remark: String
    let status: String
}

@main
struct NetPulseMobileApp: App {
    @State private var devices = [
        Device(id: 1, ip: "192.168.1.1", brand: "Huawei", remark: "Core", status: "online"),
        Device(id: 2, ip: "192.168.1.2", brand: "H3C", remark: "Access", status: "offline")
    ]
    private var onlineCount: Int { devices.filter { $0.status == "online" }.count }
    private var offlineCount: Int { devices.count - onlineCount }

    var body: some Scene {
        WindowGroup {
            NavigationStack {
                VStack(spacing: 12) {
                    HStack {
                        StatView(title: "Total", value: "\(devices.count)", color: .blue)
                        StatView(title: "Online", value: "\(onlineCount)", color: .green)
                        StatView(title: "Offline", value: "\(offlineCount)", color: .red)
                    }
                    .padding(.horizontal)

                    List(devices) { d in
                        HStack(spacing: 10) {
                            Circle().fill(d.status == "online" ? .green : .red).frame(width: 9, height: 9)
                            VStack(alignment: .leading) {
                                Text(d.ip).font(.headline)
                                Text("\(d.brand) · \(d.remark)").font(.subheadline).foregroundStyle(.secondary)
                            }
                        }
                    }
                    .refreshable { }
                }
                .navigationTitle("NetPulse")
            }
        }
    }
}

struct StatView: View {
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
