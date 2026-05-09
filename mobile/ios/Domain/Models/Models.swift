import Foundation

struct LoginRequest: Codable {
    let username: String
    let password: String
}

struct LoginResponse: Codable {
    struct User: Codable {
        let username: String
        let role: String
    }
    let token: String
    let user: User
}

struct Device: Codable, Identifiable {
    let id: Int64
    let ip: String
    let name: String
    let brand: String
    let remark: String
    let status: String
    let interfaces: [Port]
}

struct Port: Codable, Identifiable {
    let id: Int64
    let index: Int
    let name: String
    let custom_name: String?
    let remark: String
    let oper_status: Int?
}

struct DeviceHistoryPoint: Codable, Identifiable {
    var id: Date { timestamp }
    let timestamp: Date
    let cpu_usage: Double?
    let mem_usage: Double?
}

struct TrafficHistoryPoint: Codable, Identifiable {
    var id: Date { timestamp }
    let timestamp: Date
    let traffic_in_bps: Double?
    let traffic_out_bps: Double?
}

struct HistoryResponse<T: Codable>: Codable {
    let type: String
    let id: Int64
    let data: [T]
}

struct GlobalSearchItem: Codable, Identifiable {
    let type: String
    let device_id: Int64?
    let interface_id: Int64?
    let device_name: String?
    let device_ip: String?
    let interface_name: String?
    let interface_custom_name: String?
    let interface_remark: String?
    let match_field: String?
    let snippet: String?

    var id: String {
        if type == "port", let ifid = interface_id { return "p:\(ifid)" }
        if let did = device_id { return "d:\(did)" }
        return UUID().uuidString
    }
}

struct SearchResult: Identifiable, Hashable {
    enum Category: String {
        case device
        case port
    }
    let id = UUID()
    let category: Category
    let title: String
    let subtitle: String
    let deviceID: String
    let portID: String?
}
