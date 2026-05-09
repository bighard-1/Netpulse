import SwiftUI

struct LoginView: View {
    @StateObject var vm = AuthViewModel()

    var body: some View {
        VStack(spacing: 12) {
            Text("NetPulse").font(.largeTitle).bold()
            TextField("用户名", text: $vm.username).textFieldStyle(.roundedBorder)
            SecureField("密码", text: $vm.password).textFieldStyle(.roundedBorder)
            TextField("API地址", text: $vm.baseURL).textFieldStyle(.roundedBorder)
            Button(vm.loading ? "登录中..." : "登录") { Task { await vm.login() } }
                .buttonStyle(.borderedProminent)
                .disabled(vm.loading)
            if !vm.loginError.isEmpty { Text(vm.loginError).font(.footnote).foregroundStyle(.red) }
        }
        .padding(20)
    }
}
