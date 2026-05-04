package com.netpulse.mobile

import android.os.Bundle
import androidx.activity.compose.setContent
import androidx.activity.viewModels
import androidx.biometric.BiometricManager
import androidx.biometric.BiometricPrompt
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ArrowBack
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.core.content.ContextCompat
import androidx.fragment.app.FragmentActivity
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.navigation.NavType
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.rememberNavController
import androidx.navigation.navArgument
import java.time.OffsetDateTime

class MainActivity : FragmentActivity() {
    private val vm: MainViewModel by viewModels()

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContent {
            MaterialTheme {
                NetPulseApp(vm = vm, onBiometricLogin = { triggerBiometricLogin() })
            }
        }
    }

    private fun triggerBiometricLogin() {
        val biometricManager = BiometricManager.from(this)
        if (biometricManager.canAuthenticate(BiometricManager.Authenticators.BIOMETRIC_STRONG) != BiometricManager.BIOMETRIC_SUCCESS) {
            return
        }

        val executor = ContextCompat.getMainExecutor(this)
        val prompt = BiometricPrompt(this, executor, object : BiometricPrompt.AuthenticationCallback() {
            override fun onAuthenticationSucceeded(result: BiometricPrompt.AuthenticationResult) {
                val (u, p) = vm.loadSavedCreds()
                if (u.isNotBlank() && p.isNotBlank()) vm.login(u, p, rememberCreds = true)
            }
        })

        val info = BiometricPrompt.PromptInfo.Builder()
            .setTitle("生物识别登录")
            .setSubtitle("验证后自动登录 NetPulse")
            .setNegativeButtonText("取消")
            .build()
        prompt.authenticate(info)
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun NetPulseApp(vm: MainViewModel, onBiometricLogin: () -> Unit) {
    val nav = rememberNavController()
    val token by vm.token.collectAsStateWithLifecycle()
    val devices by vm.devices.collectAsStateWithLifecycle()
    val msg by vm.message.collectAsStateWithLifecycle()
    val loading by vm.loading.collectAsStateWithLifecycle()

    LaunchedEffect(token) { if (token.isNotBlank()) vm.refreshDevices() }

    val snackState = remember { SnackbarHostState() }
    LaunchedEffect(msg) {
        if (msg.isNotBlank()) snackState.showSnackbar(msg)
    }

    Scaffold(snackbarHost = { SnackbarHost(hostState = snackState) }) { _ ->
    NavHost(navController = nav, startDestination = if (token.isBlank()) "login" else "home") {
        composable("login") {
            LoginScreen(
                loading = loading,
                onLogin = { u, p -> vm.login(u, p) },
                onBio = onBiometricLogin,
                onSaveBase = { vm.saveBaseUrl(it) },
                hint = "默认地址: http://119.40.55.18:18080/api"
            )
            if (token.isNotBlank()) nav.navigate("home") { popUpTo("login") { inclusive = true } }
        }
        composable("home") {
            HomeScreen(
                devices = devices,
                loading = loading,
                onRefresh = vm::refreshDevices,
                onOpen = { id -> nav.navigate("detail/$id") },
                onLogout = vm::logout
            )
        }
        composable("detail/{id}", arguments = listOf(navArgument("id") { type = NavType.LongType })) { backStack ->
            val id = backStack.arguments?.getLong("id") ?: 0L
            val device = devices.firstOrNull { it.id == id }
            if (device == null) {
                Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) { Text("设备不存在") }
            } else {
                DetailScreen(device = device, vm = vm, onBack = { nav.popBackStack() })
            }
        }
    }
    }
}

@Composable
fun LoginScreen(loading: Boolean, onLogin: (String, String) -> Unit, onBio: () -> Unit, onSaveBase: (String) -> Unit, hint: String) {
    var u by remember { mutableStateOf("") }
    var p by remember { mutableStateOf("") }
    var base by remember { mutableStateOf("http://119.40.55.18:18080/api") }
    Column(Modifier.fillMaxSize().padding(20.dp), verticalArrangement = Arrangement.Center) {
        Text("NetPulse 移动端", style = MaterialTheme.typography.headlineSmall, fontWeight = FontWeight.Bold)
        Spacer(Modifier.height(12.dp))
        OutlinedTextField(u, { u = it }, label = { Text("用户名") }, modifier = Modifier.fillMaxWidth())
        Spacer(Modifier.height(8.dp))
        OutlinedTextField(p, { p = it }, label = { Text("密码") }, modifier = Modifier.fillMaxWidth())
        Spacer(Modifier.height(8.dp))
        OutlinedTextField(base, { base = it }, label = { Text("服务器 API 地址") }, modifier = Modifier.fillMaxWidth())
        Text(hint, style = MaterialTheme.typography.bodySmall, color = Color.Gray)
        Spacer(Modifier.height(12.dp))
        Button(onClick = { onSaveBase(base); onLogin(u, p) }, modifier = Modifier.fillMaxWidth(), enabled = !loading) { Text("登录") }
        Spacer(Modifier.height(8.dp))
        OutlinedButton(onClick = onBio, modifier = Modifier.fillMaxWidth()) { Text("Face ID / 指纹快速登录") }
        Text("首次登录必须输入用户名密码", style = MaterialTheme.typography.bodySmall, color = Color.Gray)
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun HomeScreen(devices: List<DeviceStatus>, loading: Boolean, onRefresh: () -> Unit, onOpen: (Long) -> Unit, onLogout: () -> Unit) {
    val total = devices.size
    val online = devices.count { it.status == "online" }
    val offline = total - online

    Scaffold(topBar = {
        TopAppBar(title = { Text("NetPulse 资产总览") }, actions = {
            TextButton(onClick = onRefresh) { Text("刷新") }
            TextButton(onClick = onLogout) { Text("退出") }
        })
    }) { p ->
        Column(Modifier.padding(p).fillMaxSize().padding(12.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
            Surface(tonalElevation = 2.dp, shape = MaterialTheme.shapes.medium) {
                Row(Modifier.fillMaxWidth().padding(12.dp), horizontalArrangement = Arrangement.SpaceBetween) {
                    Text("总数 $total")
                    Text("在线 $online", color = Color(0xFF2E7D32))
                    Text("离线 $offline", color = Color(0xFFC62828))
                }
            }
            if (loading) LinearProgressIndicator(Modifier.fillMaxWidth())
            LazyColumn(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                items(devices, key = { it.id }) { d ->
                    ElevatedCard(Modifier.fillMaxWidth().clickable { onOpen(d.id) }) {
                        Column(Modifier.padding(12.dp)) {
                            Row(verticalAlignment = Alignment.CenterVertically) {
                                Box(Modifier.size(10.dp)) {
                                    Surface(
                                        color = if (d.status == "online") Color(0xFF2E7D32) else Color(0xFFC62828),
                                        shape = MaterialTheme.shapes.small,
                                        modifier = Modifier.fillMaxSize()
                                    ) {}
                                }
                                Spacer(Modifier.width(8.dp))
                                Text(d.ip, style = MaterialTheme.typography.titleMedium)
                            }
                            Text("${d.brand} · ${d.remark}")
                        }
                    }
                }
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun DetailScreen(device: DeviceStatus, vm: MainViewModel, onBack: () -> Unit) {
    val logs by vm.logs.collectAsStateWithLifecycle()
    val cpu by vm.cpu.collectAsStateWithLifecycle()
    val mem by vm.mem.collectAsStateWithLifecycle()
    var start by remember { mutableStateOf(OffsetDateTime.now().minusDays(1)) }
    var end by remember { mutableStateOf(OffsetDateTime.now()) }
    var remarkDialog by remember { mutableStateOf<NetInterface?>(null) }
    var remark by remember { mutableStateOf("") }

    LaunchedEffect(device.id) { vm.loadDeviceDetail(device, start, end) }

    Scaffold(topBar = {
        TopAppBar(
            title = { Text("设备详情") },
            navigationIcon = { IconButton(onClick = onBack) { Icon(Icons.Default.ArrowBack, contentDescription = null) } },
            actions = {
                TextButton(onClick = { vm.loadDeviceDetail(device, start, end) }) { Text("刷新") }
            }
        )
    }) { p ->
        LazyColumn(Modifier.padding(p).fillMaxSize().padding(12.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
            item {
                ElevatedCard(Modifier.fillMaxWidth()) {
                    Column(Modifier.padding(12.dp)) {
                        Text(device.ip, style = MaterialTheme.typography.titleMedium)
                        Text("${device.brand} · ${device.remark}")
                        Text("CPU点数 ${cpu.size} / 内存点数 ${mem.size}")
                    }
                }
            }
            item {
                Text("端口列表", style = MaterialTheme.typography.titleMedium)
            }
            items(device.interfaces, key = { it.id }) { itf ->
                ElevatedCard(Modifier.fillMaxWidth().clickable {
                    remarkDialog = itf
                    remark = itf.remark
                }) {
                    Column(Modifier.padding(12.dp)) {
                        Text(itf.name, fontWeight = FontWeight.SemiBold)
                        Text("备注: ${itf.remark.ifBlank { "-" }}")
                        Text("点击编辑端口备注", color = Color.Gray)
                    }
                }
            }
            item { Text("最近日志", style = MaterialTheme.typography.titleMedium) }
            items(logs, key = { it.id }) { l ->
                ElevatedCard(Modifier.fillMaxWidth()) {
                    Column(Modifier.padding(12.dp)) {
                        val c = when (l.level.uppercase()) {
                            "ERROR" -> Color(0xFFC62828)
                            "WARNING" -> Color(0xFFEF6C00)
                            else -> Color(0xFF1565C0)
                        }
                        Text(l.level, color = c, fontWeight = FontWeight.Bold)
                        Text(l.message)
                        Text(l.createdAt, color = Color.Gray)
                    }
                }
            }
        }
    }

    if (remarkDialog != null) {
        AlertDialog(
            onDismissRequest = { remarkDialog = null },
            title = { Text("编辑端口备注") },
            text = {
                Column {
                    Text(remarkDialog!!.name)
                    OutlinedTextField(remark, { remark = it }, label = { Text("备注") })
                }
            },
            confirmButton = {
                TextButton(onClick = {
                    vm.updateInterfaceRemark(remarkDialog!!.id, remark) {
                        vm.refreshDevices()
                        vm.loadDeviceDetail(device, start, end)
                    }
                    remarkDialog = null
                }) { Text("保存") }
            },
            dismissButton = { TextButton(onClick = { remarkDialog = null }) { Text("取消") } }
        )
    }
}
