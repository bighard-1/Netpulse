import Foundation

struct APIClient {
    enum APIError: Error {
        case badURL
        case http(Int, String)
        case decode(String)
    }

    let baseURL: String
    let token: String

    private static let encoderISO: ISO8601DateFormatter = {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return f
    }()

    private static let decoder: JSONDecoder = {
        let d = JSONDecoder()
        d.dateDecodingStrategy = .custom { decoder in
            let c = try decoder.singleValueContainer()
            let raw = try c.decode(String.self)
            if let ts = Fmt.isoFrac.date(from: raw) ?? Fmt.iso.date(from: raw) {
                return ts
            }
            throw DecodingError.dataCorruptedError(in: c, debugDescription: "invalid RFC3339 date: \(raw)")
        }
        return d
    }()

    private func makeURL(path: String, query: [URLQueryItem] = []) throws -> URL {
        guard var comp = URLComponents(string: baseURL) else { throw APIError.badURL }
        let safeBasePath = comp.path.hasSuffix("/") ? String(comp.path.dropLast()) : comp.path
        comp.path = safeBasePath + path
        comp.queryItems = query.isEmpty ? nil : query
        guard let url = comp.url else { throw APIError.badURL }
        return url
    }

    private func request(path: String, method: String = "GET", query: [URLQueryItem] = [], body: Data? = nil) throws -> URLRequest {
        let url = try makeURL(path: path, query: query)
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
        if code == 401 {
            await MainActor.run { AuthGate.shared.handleUnauthorized() }
            throw APIError.http(401, "unauthorized")
        }
        guard (200..<300).contains(code) else {
            throw APIError.http(code, String(data: data, encoding: .utf8) ?? "http error")
        }
        return data
    }

    private func decode<T: Decodable>(_ type: T.Type, from data: Data) throws -> T {
        do {
            return try Self.decoder.decode(type, from: data)
        } catch {
            throw APIError.decode(String(describing: error))
        }
    }

    func login(username: String, password: String) async throws -> LoginResponse {
        let body = try JSONEncoder().encode(LoginRequest(username: username, password: password))
        do {
            let data = try await send(try request(path: "/auth/mobile/login", method: "POST", body: body))
            return try decode(LoginResponse.self, from: data)
        } catch {
            let data = try await send(try request(path: "/login", method: "POST", body: body))
            return try decode(LoginResponse.self, from: data)
        }
    }

    func fetchDevices() async throws -> [Device] {
        let data = try await send(try request(path: "/devices"))
        return try decode([Device].self, from: data)
    }

    func fetchDevice(id: String) async throws -> Device {
        let data = try await send(try request(path: "/devices/\(id)"))
        return try decode(Device.self, from: data)
    }

    func search(keyword: String) async throws -> [GlobalSearchItem] {
        let q = keyword.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !q.isEmpty else { return [] }
        let data = try await send(try request(path: "/search", query: [URLQueryItem(name: "q", value: q)]))
        return try decode([GlobalSearchItem].self, from: data)
    }

    func fetchDeviceHistory(type: String, id: String, start: Date, end: Date, maxPoints: Int, interval: String) async throws -> [DeviceHistoryPoint] {
        let query = [
            URLQueryItem(name: "type", value: type),
            URLQueryItem(name: "id", value: id),
            URLQueryItem(name: "start", value: Self.encoderISO.string(from: start)),
            URLQueryItem(name: "end", value: Self.encoderISO.string(from: end)),
            URLQueryItem(name: "max_points", value: String(maxPoints)),
            URLQueryItem(name: "interval", value: interval)
        ]
        let data = try await send(try request(path: "/metrics/history", query: query))
        return try decode(HistoryResponse<DeviceHistoryPoint>.self, from: data).data
    }

    func fetchTraffic(id: String, start: Date, end: Date, maxPoints: Int, interval: String) async throws -> [TrafficHistoryPoint] {
        let query = [
            URLQueryItem(name: "type", value: "traffic"),
            URLQueryItem(name: "id", value: id),
            URLQueryItem(name: "start", value: Self.encoderISO.string(from: start)),
            URLQueryItem(name: "end", value: Self.encoderISO.string(from: end)),
            URLQueryItem(name: "max_points", value: String(maxPoints)),
            URLQueryItem(name: "interval", value: interval)
        ]
        let data = try await send(try request(path: "/metrics/history", query: query))
        return try decode(HistoryResponse<TrafficHistoryPoint>.self, from: data).data
    }
}
