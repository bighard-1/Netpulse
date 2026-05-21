import SwiftUI

struct StatusDot: View {
    enum Kind {
        case up
        case down
        case disabled
        case unknown
    }

    private let kind: Kind

    init(status: Int?) {
        switch status {
        case 1: self.kind = .up
        case 2: self.kind = .down
        default: self.kind = .unknown
        }
    }

    init(status: Int?, adminStatus: Int?) {
        if adminStatus == 2 {
            self.kind = .disabled
            return
        }
        switch status {
        case 1: self.kind = .up
        case 2: self.kind = .down
        default: self.kind = .unknown
        }
    }

    init(status: String) {
        switch status.lowercased() {
        case "online", "up": self.kind = .up
        case "offline", "down": self.kind = .down
        default: self.kind = .unknown
        }
    }

    var body: some View {
        symbol
            .accessibilityLabel(label)
    }

    @ViewBuilder
    private var symbol: some View {
        switch kind {
        case .up:
            Circle()
                .fill(Color(red: 0.05, green: 0.12, blue: 0.09))
                .frame(width: 17, height: 17)
                .overlay(Circle().fill(Color(red: 0.0, green: 0.86, blue: 0.35)).frame(width: 11, height: 11))
                .shadow(color: Color.green.opacity(0.35), radius: 3)
        case .down:
            Circle()
                .fill(Color(red: 0.14, green: 0.05, blue: 0.05))
                .frame(width: 17, height: 17)
                .overlay(Circle().fill(Color(red: 0.95, green: 0.08, blue: 0.12)).frame(width: 11, height: 11))
                .shadow(color: Color.red.opacity(0.35), radius: 3)
        case .disabled:
            ZStack {
                Circle()
                    .fill(Color(red: 0.18, green: 0.04, blue: 0.05))
                    .frame(width: 18, height: 18)
                Image(systemName: "xmark")
                    .font(.system(size: 10, weight: .heavy))
                    .foregroundStyle(Color(red: 1.0, green: 0.16, blue: 0.18))
            }
            .shadow(color: Color.red.opacity(0.35), radius: 3)
        case .unknown:
            Circle()
                .fill(Color(red: 0.14, green: 0.12, blue: 0.05))
                .frame(width: 17, height: 17)
                .overlay(Circle().fill(Color(red: 0.95, green: 0.72, blue: 0.12)).frame(width: 11, height: 11))
        }
    }

    private var label: String {
        switch kind {
        case .up: return "UP"
        case .down: return "DOWN"
        case .disabled: return "ADMIN DOWN"
        case .unknown: return "UNKNOWN"
        }
    }
}
