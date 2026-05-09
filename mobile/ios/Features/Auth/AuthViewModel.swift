import Foundation

@MainActor
final class AuthViewModel: ObservableObject {
    @Published var username = ""
    @Published var password = ""
    @Published var baseURL = UserDefaults.standard.string(forKey: "np.base") ?? "http://119.40.55.18:18080/api"
    @Published var loginError = ""
    @Published var loading = false

    func login() async {
        loading = true
        defer { loading = false }
        do {
            let resp = try await APIClient(baseURL: normalizedBase(baseURL), token: "").login(username: username, password: password)
            KeychainManager.saveToken(resp.token)
            UserDefaults.standard.set(normalizedBase(baseURL), forKey: "np.base")
            loginError = ""
            AuthGate.shared.setAuthenticated(true)
        } catch {
            loginError = readableError(error)
        }
    }

    private func normalizedBase(_ raw: String) -> String {
        var v = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        if v.hasSuffix("/") { v.removeLast() }
        return v
    }

    private func readableError(_ error: Error) -> String {
        let msg = String(describing: error)
        if msg.contains("-1022") || msg.localizedCaseInsensitiveContains("App Transport Security") {
            return "网络策略阻止了不安全连接，请确认使用 http:// 内网地址且客户端已启用ATS例外。"
        }
        if msg.contains("401") {
            return "登录失败：账号或密码错误。"
        }
        return "登录失败：\(msg)"
    }
}
