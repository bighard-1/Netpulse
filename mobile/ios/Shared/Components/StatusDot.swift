import SwiftUI

struct StatusDot: View {
    private let color: Color

    init(status: Int?) {
        switch status {
        case 1: self.color = .green
        case 2: self.color = .red
        default: self.color = .yellow
        }
    }

    init(status: String) {
        switch status.lowercased() {
        case "online", "up": self.color = .green
        case "offline", "down": self.color = .red
        default: self.color = .yellow
        }
    }

    var body: some View {
        Circle()
            .fill(color)
            .frame(width: 11, height: 11)
            .shadow(color: color.opacity(0.6), radius: 4)
    }
}
