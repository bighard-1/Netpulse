import SwiftUI

struct StatefulContainer<T, Content: View>: View {
    let state: ViewState<T>
    let retry: (() -> Void)?
    let content: (T) -> Content

    var body: some View {
        switch state {
        case .loading:
            ProgressView("加载中...")
                .frame(maxWidth: .infinity, maxHeight: .infinity)
        case .error(let msg):
            VStack(spacing: 10) {
                Text(msg).font(.footnote).foregroundStyle(.red)
                if let retry { Button("重试", action: retry) }
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity)
        case .success(let val):
            content(val)
        }
    }
}
