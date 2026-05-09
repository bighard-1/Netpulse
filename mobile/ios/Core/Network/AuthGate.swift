import Foundation

@MainActor
final class AuthGate: ObservableObject {
    static let shared = AuthGate()
    @Published private(set) var sessionState: SessionState = .unauthenticated
    private var didExpire = false

    func setAuthenticated(_ value: Bool) {
        sessionState = value ? .authenticated : .unauthenticated
        if !value { didExpire = false }
    }

    func handleUnauthorized() {
        guard !didExpire else { return }
        didExpire = true
        KeychainManager.clearToken()
        sessionState = .expired
    }

    func resetExpiredToLogin() {
        sessionState = .unauthenticated
        didExpire = false
    }
}
