import Foundation

enum ViewState<T> {
    case loading
    case success(T)
    case error(String)
}
