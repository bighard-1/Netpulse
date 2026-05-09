import SwiftUI

@main
struct NetPulseApp: App {
    @StateObject private var gate = AuthGate.shared

    var body: some Scene {
        WindowGroup {
            Group {
                switch gate.sessionState {
                case .unauthenticated, .expired:
                    LoginView()
                        .onAppear {
                            if gate.sessionState == .expired { gate.resetExpiredToLogin() }
                        }
                case .authenticated:
                    DeviceListView()
                }
            }
            .task {
                let hasToken = !KeychainManager.loadToken().isEmpty
                gate.setAuthenticated(hasToken)
            }
        }
    }
}
