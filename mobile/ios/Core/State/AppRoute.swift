import Foundation

enum AppRoute: Hashable {
    case device(String)
    case port(deviceID: String, portID: String)
}
