import SwiftUI

struct StatusDot: View {
    enum Kind {
        case up
        case down
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

    init(status: String) {
        switch status.lowercased() {
        case "online", "up": self.kind = .up
        case "offline", "down": self.kind = .down
        default: self.kind = .unknown
        }
    }

    var body: some View {
        HStack(spacing: 4) {
            symbol
            Text(label)
                .font(.caption2)
                .foregroundStyle(.secondary)
        }
    }

    @ViewBuilder
    private var symbol: some View {
        switch kind {
        case .up:
            Circle()
                .fill(Color.green)
                .frame(width: 11, height: 11)
                .overlay(Image(systemName: "checkmark").font(.system(size: 7, weight: .bold)).foregroundStyle(.white))
        case .down:
            RoundedRectangle(cornerRadius: 2)
                .fill(Color.red)
                .frame(width: 11, height: 11)
                .overlay(Image(systemName: "xmark").font(.system(size: 7, weight: .bold)).foregroundStyle(.white))
        case .unknown:
            Triangle()
                .fill(Color.yellow)
                .frame(width: 12, height: 11)
                .overlay(Image(systemName: "questionmark").font(.system(size: 7, weight: .bold)).foregroundStyle(.black))
        }
    }

    private var label: String {
        switch kind {
        case .up: return "UP"
        case .down: return "DOWN"
        case .unknown: return "UNKNOWN"
        }
    }
}

private struct Triangle: Shape {
    func path(in rect: CGRect) -> Path {
        var path = Path()
        path.move(to: CGPoint(x: rect.midX, y: rect.minY))
        path.addLine(to: CGPoint(x: rect.maxX, y: rect.maxY))
        path.addLine(to: CGPoint(x: rect.minX, y: rect.maxY))
        path.closeSubpath()
        return path
    }
}
