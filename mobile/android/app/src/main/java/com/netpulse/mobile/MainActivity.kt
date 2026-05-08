package com.netpulse.mobile

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.os.Bundle
import android.widget.TextView
import androidx.activity.compose.setContent
import androidx.activity.viewModels
import androidx.biometric.BiometricManager
import androidx.biometric.BiometricPrompt
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.core.*
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.ExperimentalMaterialApi
import androidx.compose.material.pullrefresh.PullRefreshIndicator
import androidx.compose.material.pullrefresh.pullRefresh
import androidx.compose.material.pullrefresh.rememberPullRefreshState
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Home
import androidx.compose.material.icons.filled.Person
import androidx.compose.material3.*
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.core.content.ContextCompat
import androidx.fragment.app.FragmentActivity
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.navigation.NavType
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.navigation.compose.rememberNavController
import androidx.navigation.navArgument
import com.github.mikephil.charting.charts.LineChart
import com.github.mikephil.charting.components.MarkerView
import com.github.mikephil.charting.components.XAxis
import com.github.mikephil.charting.data.Entry
import com.github.mikephil.charting.data.LineData
import com.github.mikephil.charting.data.LineDataSet
import com.github.mikephil.charting.formatter.ValueFormatter
import com.github.mikephil.charting.highlight.Highlight
import com.github.mikephil.charting.utils.MPPointF
import java.time.OffsetDateTime
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.util.Date

private object Np {
    val Bg = Color(0xFF0F172A)
    val Indigo = Color(0xFF6366F1)
    val Success = Color(0xFF10B981)
    val Danger = Color(0xFFEF4444)
    val Warning = Color(0xFFF59E0B)
    val screenPadding = 16.dp
    val sectionGap = 12.dp
    val cardPadding = 14.dp
    val corner = 12.dp
}

class MainActivity : FragmentActivity() {
    private val vm: MainViewModel by viewModels()

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContent {
            MaterialTheme(colorScheme = darkColorScheme(primary = Np.Indigo, background = Np.Bg, surface = Color(0xFF1E293B))) {
                Surface(color = Np.Bg) {
                    NetPulseApp(vm = vm, onBiometricLogin = { triggerBiometricLogin() })
                }
            }
        }
    }

    private fun triggerBiometricLogin() {
        val biometricManager = BiometricManager.from(this)
        if (biometricManager.canAuthenticate(BiometricManager.Authenticators.BIOMETRIC_STRONG) != BiometricManager.BIOMETRIC_SUCCESS) return
        val executor = ContextCompat.getMainExecutor(this)
        val prompt = BiometricPrompt(this, executor, object : BiometricPrompt.AuthenticationCallback() {
            override fun onAuthenticationSucceeded(result: BiometricPrompt.AuthenticationResult) {
                vm.biometricUnlock()
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

@Composable
fun NetPulseApp(vm: MainViewModel, onBiometricLogin: () -> Unit) {
    val nav = rememberNavController()
    val token by vm.token.collectAsStateWithLifecycle()
    val msg by vm.message.collectAsStateWithLifecycle()
    val loading by vm.loading.collectAsStateWithLifecycle()
    val snackState = remember { SnackbarHostState() }

    LaunchedEffect(msg) { if (msg.isNotBlank()) snackState.showSnackbar(msg) }
    LaunchedEffect(token) {
        if (token.isBlank()) nav.navigate("login") { popUpTo(0) }
        else if (nav.currentDestination?.route == "login") nav.navigate("home") { popUpTo("login") { inclusive = true } }
    }

    Scaffold(snackbarHost = { SnackbarHost(hostState = snackState) }) { p ->
        Box(Modifier.padding(p)) {
            NavHost(navController = nav, startDestination = if (token.isBlank()) "login" else "home") {
                composable("login") {
                    LoginScreen(
                        loading = loading,
                        onLogin = { u, pw -> vm.login(u, pw) },
                        onBio = onBiometricLogin,
                        onSaveBase = vm::saveBaseUrl,
                        hint = "默认地址: http://119.40.55.18:18080/api"
                    )
                }
                composable("home") { MainShell(vm = vm, nav = nav) }
                composable("device/{id}", arguments = listOf(navArgument("id") { type = NavType.LongType })) {
                    val id = it.arguments?.getLong("id") ?: 0L
                    DeviceDetailScreen(id, vm, onBack = { nav.popBackStack() }, onOpenPort = { portId -> nav.navigate("port/$portId") })
                }
                composable("port/{id}", arguments = listOf(navArgument("id") { type = NavType.LongType })) {
                    val id = it.arguments?.getLong("id") ?: 0L
                    PortDetailScreen(id, vm, onBack = { nav.popBackStack() })
                }
            }
        }
    }
}

@Composable
private fun MainShell(vm: MainViewModel, nav: androidx.navigation.NavHostController) {
    val devices by vm.devices.collectAsStateWithLifecycle()
    val loading by vm.loading.collectAsStateWithLifecycle()
    val auditLogs by vm.auditLogs.collectAsStateWithLifecycle()
    var tab by remember { mutableStateOf("dashboard") }

    Scaffold(
        bottomBar = {
            NavigationBar {
                NavigationBarItem(
                    selected = tab == "dashboard",
                    onClick = { tab = "dashboard" },
                    icon = { Icon(Icons.Default.Home, contentDescription = null) },
                    label = { Text("仪表盘") }
                )
                NavigationBarItem(
                    selected = tab == "assets",
                    onClick = { tab = "assets" },
                    icon = { Icon(Icons.Default.Home, contentDescription = null) },
                    label = { Text("资产中心") }
                )
                NavigationBarItem(
                    selected = tab == "me",
                    onClick = { tab = "me" },
                    icon = { Icon(Icons.Default.Person, contentDescription = null) },
                    label = { Text("我的") }
                )
            }
        }
    ) { p ->
        when (tab) {
            "dashboard" -> HomeScreen(
                title = "仪表盘",
                vm = vm,
                devices = devices,
                loading = loading,
                recentEvents = auditLogs,
                onRefresh = vm::refreshDevices,
                onOpen = { id -> nav.navigate("device/$id") },
                onQuickPeek = { id -> vm.openQuickPeek(id) },
                modifier = Modifier.padding(p)
            )
            "assets" -> HomeScreen(
                title = "资产中心（只读）",
                vm = vm,
                devices = devices,
                loading = loading,
                recentEvents = auditLogs,
                onRefresh = vm::refreshDevices,
                onOpen = { id -> nav.navigate("device/$id") },
                onQuickPeek = { id -> vm.openQuickPeek(id) },
                modifier = Modifier.padding(p)
            )
            else -> ProfileScreen(vm = vm, modifier = Modifier.padding(p))
        }
    }
}

@Composable
private fun ProfileScreen(vm: MainViewModel, modifier: Modifier = Modifier) {
    Column(
        modifier.fillMaxSize().padding(Np.screenPadding),
        verticalArrangement = Arrangement.spacedBy(12.dp)
    ) {
        Text("NetPulse", style = MaterialTheme.typography.headlineSmall, fontWeight = FontWeight.Bold)
        Text("移动端设置", color = Color.Gray)
        OutlinedButton(onClick = { vm.logout() }) { Text("退出登录") }
    }
}

@Composable
fun LoginScreen(loading: Boolean, onLogin: (String, String) -> Unit, onBio: () -> Unit, onSaveBase: (String) -> Unit, hint: String) {
    var u by remember { mutableStateOf("") }
    var p by remember { mutableStateOf("") }
    var base by remember { mutableStateOf("http://119.40.55.18:18080/api") }
    Column(Modifier.fillMaxSize().padding(Np.screenPadding), verticalArrangement = Arrangement.Center) {
        Text("NetPulse", style = MaterialTheme.typography.headlineMedium, fontWeight = FontWeight.Bold)
        Text("移动端只读工作台", color = Color.Gray)
        Spacer(Modifier.height(Np.sectionGap))
        OutlinedTextField(u, { u = it }, label = { Text("用户名") }, modifier = Modifier.fillMaxWidth())
        Spacer(Modifier.height(8.dp))
        OutlinedTextField(p, { p = it }, label = { Text("密码") }, modifier = Modifier.fillMaxWidth())
        Spacer(Modifier.height(8.dp))
        OutlinedTextField(base, { base = it }, label = { Text("服务器 API 地址") }, modifier = Modifier.fillMaxWidth())
        Text(hint, style = MaterialTheme.typography.bodySmall, color = Color.Gray)
        Spacer(Modifier.height(Np.sectionGap))
        Button(onClick = { onSaveBase(base); onLogin(u, p) }, modifier = Modifier.fillMaxWidth(), enabled = !loading) { Text("登录") }
        Spacer(Modifier.height(8.dp))
        OutlinedButton(onClick = onBio, modifier = Modifier.fillMaxWidth()) { Text("Face ID / Touch ID") }
        Text("首次登录必须输入用户名密码", style = MaterialTheme.typography.bodySmall, color = Color.Gray)
    }
}

@OptIn(ExperimentalFoundationApi::class, ExperimentalMaterial3Api::class)
@Composable
fun HomeScreen(
    title: String,
    vm: MainViewModel,
    devices: List<DeviceStatus>,
    loading: Boolean,
    recentEvents: List<AuditLog>,
    onRefresh: () -> Unit,
    onOpen: (Long) -> Unit,
    onQuickPeek: (Long) -> Unit,
    modifier: Modifier = Modifier
) {
    val total = devices.size
    val online = devices.count { it.status == "online" }
    val offline = devices.count { it.status == "offline" || it.status == "unknown" }
    val healthScore = (online.toFloat() / (total.coerceAtLeast(1)).toFloat() * 100f).toInt()
    val ctx = LocalContext.current
    val todoItems = buildList {
        if (devices.isEmpty()) add("添加首台资产（请在 Web 端资产中心操作）")
        if (offline > 0) add("排查离线/未知资产：$offline 台")
        if (recentEvents.isNotEmpty()) add("检查最新事件并确认是否需要处置")
    }

    Column(modifier.fillMaxSize().padding(Np.screenPadding), verticalArrangement = Arrangement.spacedBy(Np.sectionGap)) {
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween, verticalAlignment = Alignment.CenterVertically) {
            Text(title, style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.SemiBold)
            AssistChip(onClick = {}, label = { Text("仅查看") })
        }
        Row(Modifier.horizontalScroll(rememberScrollState()), horizontalArrangement = Arrangement.spacedBy(10.dp)) {
            MiniStatCard("总数", "$total", Brush.linearGradient(listOf(Color(0xFF334155), Color(0xFF1E293B))))
            MiniStatCard("在线", "$online", Brush.linearGradient(listOf(Color(0xFF0F766E), Color(0xFF065F46))), showPulse = true)
            MiniStatCard("离线", "$offline", Brush.linearGradient(listOf(Color(0xFF991B1B), Color(0xFF7F1D1D))))
            MiniStatCard("健康度", "$healthScore", Brush.linearGradient(listOf(Color(0xFF4338CA), Color(0xFF6366F1))))
        }
        if (todoItems.isNotEmpty()) {
            ElevatedCard(shape = RoundedCornerShape(Np.corner), colors = CardDefaults.elevatedCardColors(containerColor = Color(0xFF1E293B))) {
                Column(Modifier.fillMaxWidth().padding(Np.cardPadding), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Text("今日待处理", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold)
                    todoItems.take(3).forEach { t ->
                        Text("• $t", style = MaterialTheme.typography.bodySmall, color = Color(0xFFCBD5E1))
                    }
                }
            }
        }

        if (loading) {
            repeat(3) { SkeletonCard() }
        } else if (devices.isEmpty()) {
            EmptyStateCard(title = "暂无资产", desc = "请先在 Web 端添加资产后再查看")
        } else {
            LazyColumn(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                items(devices, key = { it.id }) { d ->
                    ElevatedCard(
                        modifier = Modifier.fillMaxWidth().combinedClickable(
                            onClick = { onOpen(d.id) },
                            onLongClick = { onQuickPeek(d.id) }
                        ),
                        shape = RoundedCornerShape(Np.corner),
                        colors = CardDefaults.elevatedCardColors(containerColor = Color(0xFF1E293B))
                    ) {
                        Column(Modifier.padding(Np.cardPadding)) {
                            Row(verticalAlignment = Alignment.CenterVertically) {
                                PulseDot(d.status)
                                Spacer(Modifier.width(8.dp))
                                Text(
                                    d.ip,
                                    style = MaterialTheme.typography.titleMedium,
                                    modifier = Modifier.combinedClickable(onClick = {}, onLongClick = { copyToClipboard(ctx, d.ip) })
                                )
                            }
                            Text("${statusText(d.status)} · ${d.brand} · ${d.remark.ifBlank { "未备注" }}", style = MaterialTheme.typography.bodyMedium)
                            if (!d.statusReason.isNullOrBlank()) Text(d.statusReason, style = MaterialTheme.typography.bodySmall, color = Color(0xFF94A3B8))
                        }
                    }
                }
            }
        }

        ElevatedCard(shape = RoundedCornerShape(Np.corner), colors = CardDefaults.elevatedCardColors(containerColor = Color(0xFF1E293B))) {
            Column(Modifier.fillMaxWidth().padding(Np.cardPadding), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
                    Text("系统实时事件流", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold)
                    TextButton(onClick = onRefresh) { Text("刷新") }
                }
                if (recentEvents.isEmpty()) {
                    Text("暂无关键事件（Web 端配置告警后将显示）", color = Color.Gray)
                } else {
                    recentEvents.take(3).forEach { event ->
                        val sev = severityOf(event)
                        val color = when (sev) {
                            "error" -> Np.Danger
                            "warning" -> Np.Warning
                            else -> Np.Success
                        }
                        Text("[${sev.uppercase()}] ${event.action} · ${event.target ?: "-"}", color = color, style = MaterialTheme.typography.bodySmall)
                    }
                }
            }
        }
    }

    val quickPeekDevice by vm.quickPeekDevice.collectAsStateWithLifecycle()
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = false)
    if (quickPeekDevice != null) {
        ModalBottomSheet(onDismissRequest = { vm.closeQuickPeek() }, sheetState = sheetState) {
            DeviceQuickPeekSheet(vm = vm, device = quickPeekDevice!!, onOpenDetail = onOpen)
        }
    }

}

@OptIn(ExperimentalMaterialApi::class, ExperimentalFoundationApi::class, ExperimentalMaterial3Api::class)
@Composable
fun DeviceDetailScreen(deviceId: Long, vm: MainViewModel, onBack: () -> Unit, onOpenPort: (Long) -> Unit) {
    val device by vm.deviceDetail.collectAsStateWithLifecycle()
    val cpu by vm.cpu.collectAsStateWithLifecycle()
    val mem by vm.mem.collectAsStateWithLifecycle()
    val logs by vm.logs.collectAsStateWithLifecycle()
    val loading by vm.loading.collectAsStateWithLifecycle()
    var keyword by remember { mutableStateOf("") }
    var showLogs by remember { mutableStateOf(false) }
    var start by remember { mutableStateOf(OffsetDateTime.now().minusDays(1)) }
    var end by remember { mutableStateOf(OffsetDateTime.now()) }
    val ctx = LocalContext.current

    LaunchedEffect(deviceId) { vm.loadDeviceDetail(deviceId, start, end) }
    val ports = device?.interfaces.orEmpty().filter {
        val k = keyword.lowercase().trim()
        if (k.isBlank()) true else "${it.id} ${it.index} ${it.name} ${it.remark}".lowercase().contains(k)
    }
    val refreshState = rememberPullRefreshState(
        refreshing = loading,
        onRefresh = { vm.loadDeviceDetail(deviceId, start, end) }
    )
    val cpuVals = cpu.map { it.cpuUsage ?: 0.0 }
    val memVals = mem.map { it.memUsage ?: 0.0 }
    val cpuCurrent = cpuVals.lastOrNull() ?: 0.0
    val memCurrent = memVals.lastOrNull() ?: 0.0
    val cpuPeak = cpuVals.maxOrNull() ?: 0.0
    val memPeak = memVals.maxOrNull() ?: 0.0

    Scaffold(topBar = {
        TopAppBar(
            title = { Text("设备详情") },
            navigationIcon = { IconButton(onClick = onBack) { Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = null) } }
        )
    }) { p ->
        Box(Modifier.padding(p).fillMaxSize().pullRefresh(refreshState)) {
            LazyColumn(Modifier.fillMaxSize().padding(Np.screenPadding), verticalArrangement = Arrangement.spacedBy(Np.sectionGap)) {
                item {
                    ElevatedCard(shape = RoundedCornerShape(Np.corner), colors = CardDefaults.elevatedCardColors(containerColor = Color(0xFF1E293B))) {
                        Column(Modifier.padding(Np.cardPadding)) {
                            Row(verticalAlignment = Alignment.CenterVertically) {
                                PulseDot(device?.status ?: "unknown")
                                Spacer(Modifier.width(8.dp))
                                Text(device?.ip ?: "-", fontWeight = FontWeight.SemiBold, modifier = Modifier.combinedClickable(onClick = {}, onLongClick = { copyToClipboard(ctx, device?.ip ?: "") }))
                            }
                            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween, verticalAlignment = Alignment.CenterVertically) {
                                Text("${device?.brand ?: "-"} · ${device?.remark ?: "-"}")
                                AssistChip(onClick = {}, label = { Text("只读模式") })
                            }
                        }
                    }
                }
                item {
                    ElevatedCard(shape = RoundedCornerShape(Np.corner), colors = CardDefaults.elevatedCardColors(containerColor = Color(0xFF1E293B))) {
                        Column(Modifier.padding(Np.cardPadding)) {
                            Text("CPU / 内存", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold)
                            Spacer(Modifier.height(6.dp))
                            Text("CPU 当前 ${"%.1f".format(cpuCurrent)}% / 峰值 ${"%.1f".format(cpuPeak)}%", style = MaterialTheme.typography.bodySmall, color = Color.Gray)
                            Text("内存 当前 ${"%.1f".format(memCurrent)}% / 峰值 ${"%.1f".format(memPeak)}%", style = MaterialTheme.typography.bodySmall, color = Color.Gray)
                            Spacer(Modifier.height(8.dp))
                            if (loading) SkeletonBox(Modifier.fillMaxWidth().height(220.dp))
                            else MiniLineChart(cpuVals, memVals)
                        }
                    }
                }
                item { OutlinedTextField(keyword, { keyword = it }, label = { Text("搜索端口") }, modifier = Modifier.fillMaxWidth()) }
                if (loading) {
                    items(3) { SkeletonCard() }
                } else {
                    items(ports, key = { it.id }) { itf ->
                        ElevatedCard(
                            modifier = Modifier.fillMaxWidth().combinedClickable(
                                onClick = { onOpenPort(itf.id) },
                                onLongClick = { onOpenPort(itf.id) }
                            ),
                            shape = RoundedCornerShape(Np.corner),
                            colors = CardDefaults.elevatedCardColors(containerColor = Color(0xFF1E293B))
                        ) {
                            Column(Modifier.padding(Np.cardPadding)) {
                                Text(itf.name, fontWeight = FontWeight.SemiBold)
                                Text("索引: ${itf.index} · 备注: ${itf.remark.ifBlank { "-" }}")
                            }
                        }
                    }
                    if (ports.isEmpty()) {
                        item { EmptyStateCard(title = "无匹配端口", desc = "请调整关键字后再试") }
                    }
                }
                item {
                    ElevatedCard(shape = RoundedCornerShape(Np.corner), colors = CardDefaults.elevatedCardColors(containerColor = Color(0xFF1E293B))) {
                        Column(Modifier.padding(Np.cardPadding), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween, verticalAlignment = Alignment.CenterVertically) {
                                Text("设备日志（默认10条）", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold)
                                TextButton(onClick = { showLogs = !showLogs }) { Text(if (showLogs) "收起" else "展开") }
                            }
                            AnimatedVisibility(visible = showLogs) {
                                Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                                    logs.take(10).forEach { log ->
                                        val c = when (log.level.uppercase()) {
                                            "ERROR" -> Np.Danger
                                            "WARNING", "WARN" -> Np.Warning
                                            else -> Np.Success
                                        }
                                        Text("[${log.level}] ${log.message}", color = c, style = MaterialTheme.typography.bodySmall)
                                    }
                                }
                            }
                        }
                    }
                }
            }
            PullRefreshIndicator(loading, refreshState, Modifier.align(Alignment.TopCenter))
        }
    }

}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PortDetailScreen(portId: Long, vm: MainViewModel, onBack: () -> Unit) {
    val traffic by vm.traffic.collectAsStateWithLifecycle()
    val loading by vm.loading.collectAsStateWithLifecycle()
    var start by remember { mutableStateOf(OffsetDateTime.now().minusDays(1)) }
    var end by remember { mutableStateOf(OffsetDateTime.now()) }
    var showCustomRange by remember { mutableStateOf(false) }

    LaunchedEffect(portId) { vm.loadPortTraffic(portId, start, end) }

    Scaffold(topBar = {
        TopAppBar(
            title = { Text("端口流量") },
            navigationIcon = { IconButton(onClick = onBack) { Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = null) } },
            actions = { TextButton(onClick = { vm.loadPortTraffic(portId, start, end) }) { Text("刷新") } }
        )
    }) { p ->
        Column(Modifier.padding(p).fillMaxSize().padding(Np.screenPadding), verticalArrangement = Arrangement.spacedBy(Np.sectionGap)) {
            ElevatedCard(shape = RoundedCornerShape(Np.corner), colors = CardDefaults.elevatedCardColors(containerColor = Color(0xFF1E293B)), modifier = Modifier.fillMaxWidth()) {
                Column(Modifier.fillMaxWidth().padding(Np.cardPadding), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Text("时间范围", style = MaterialTheme.typography.titleMedium, color = Color.White)
                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.horizontalScroll(rememberScrollState())) {
                        OutlinedButton(onClick = {
                            end = OffsetDateTime.now()
                            start = end.minusDays(1)
                            vm.loadPortTraffic(portId, start, end)
                        }) { Text("当日") }
                        OutlinedButton(onClick = {
                            end = OffsetDateTime.now()
                            start = end.minusDays(7)
                            vm.loadPortTraffic(portId, start, end)
                        }) { Text("近7天") }
                        OutlinedButton(onClick = {
                            end = OffsetDateTime.now()
                            start = end.minusDays(30)
                            vm.loadPortTraffic(portId, start, end)
                        }) { Text("近30天") }
                        OutlinedButton(onClick = {
                            end = OffsetDateTime.now()
                            start = end.minusDays(365 * 3L)
                            vm.loadPortTraffic(portId, start, end)
                        }) { Text("近3年") }
                    }
                    TextButton(onClick = { showCustomRange = !showCustomRange }) {
                        Text(if (showCustomRange) "收起自定义时间" else "展开自定义时间")
                    }
                    AnimatedVisibility(visible = showCustomRange) {
                        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                            Text("开始: ${start}", style = MaterialTheme.typography.bodySmall, color = Color.Gray)
                            Text("结束: ${end}", style = MaterialTheme.typography.bodySmall, color = Color.Gray)
                            Text("提示: 当前版本请先用预设范围快速查询；自定义精确选择将在下轮补充日期时间选择器。", style = MaterialTheme.typography.bodySmall, color = Np.Warning)
                            Button(onClick = { vm.loadPortTraffic(portId, start, end) }, modifier = Modifier.fillMaxWidth()) {
                                Text("按当前范围查询")
                            }
                        }
                    }
                }
            }
            ElevatedCard(shape = RoundedCornerShape(Np.corner), colors = CardDefaults.elevatedCardColors(containerColor = Color(0xFF1E293B)), modifier = Modifier.fillMaxWidth().weight(1f)) {
                if (loading) {
                    SkeletonBox(Modifier.fillMaxSize().padding(12.dp))
                } else if (traffic.isEmpty()) {
                    EmptyStateCard(title = "暂无流量数据", desc = "请调整时间范围后刷新", modifier = Modifier.fillMaxSize())
                } else {
                    val decimated = remember(traffic) { decimateTraffic(traffic, 1800) }
                    MpTrafficChart(points = decimated, modifier = Modifier.fillMaxSize().padding(10.dp))
                }
            }
        }
    }
}

@Composable
fun MpTrafficChart(points: List<InterfaceHistoryPoint>, modifier: Modifier = Modifier) {
    val formatter = DateTimeFormatter.ofPattern("MM-dd HH:mm")
    AndroidView(modifier = modifier, factory = { ctx ->
        LineChart(ctx).apply {
            description.isEnabled = false
            setTouchEnabled(true)
            setPinchZoom(true)
            isDragEnabled = true
            setScaleEnabled(true)
            setVisibleXRangeMaximum(180f)
            axisRight.isEnabled = false
            xAxis.position = XAxis.XAxisPosition.BOTTOM
            xAxis.granularity = 1f
            xAxis.valueFormatter = object : ValueFormatter() {
                override fun getFormattedValue(value: Float): String {
                    val i = value.toInt().coerceIn(0, points.lastIndex)
                    return parseTs(points[i].timestamp).format(formatter)
                }
            }
            marker = TrafficMarkerView(ctx, points)
        }
    }, update = { chart ->
        if (points.isEmpty()) {
            chart.clear()
            return@AndroidView
        }
        val inEntries = points.mapIndexed { i, p -> Entry(i.toFloat(), (p.trafficInBps ?: 0.0).toFloat()) }
        val outEntries = points.mapIndexed { i, p -> Entry(i.toFloat(), (p.trafficOutBps ?: 0.0).toFloat()) }

        val inSet = LineDataSet(inEntries, "入方向").apply {
            color = android.graphics.Color.parseColor("#6366F1")
            setDrawCircles(false)
            lineWidth = 2f
            mode = LineDataSet.Mode.LINEAR
        }
        val outSet = LineDataSet(outEntries, "出方向").apply {
            color = android.graphics.Color.parseColor("#22C55E")
            setDrawCircles(false)
            lineWidth = 2f
            mode = LineDataSet.Mode.LINEAR
        }
        chart.data = LineData(inSet, outSet)
        chart.invalidate()
    })
}

class TrafficMarkerView(context: Context, private val points: List<InterfaceHistoryPoint>) : MarkerView(context, R.layout.chart_marker) {
    private val tv: TextView = findViewById(R.id.markerText)
    override fun refreshContent(e: Entry?, highlight: Highlight?) {
        if (e == null || points.isEmpty()) return
        val i = e.x.toInt().coerceIn(0, points.lastIndex)
        val p = points[i]
        tv.text = "${p.timestamp}\n入: ${(p.trafficInBps ?: 0.0).toLong()} bps\n出: ${(p.trafficOutBps ?: 0.0).toLong()} bps"
        super.refreshContent(e, highlight)
    }
    override fun getOffset(): MPPointF = MPPointF(-(width / 2f), -height.toFloat())
}

fun decimateTraffic(src: List<InterfaceHistoryPoint>, maxPoints: Int): List<InterfaceHistoryPoint> {
    if (src.size <= maxPoints) return src
    val bucket = src.size.toDouble() / maxPoints
    val out = ArrayList<InterfaceHistoryPoint>(maxPoints)
    var i = 0.0
    while (i < src.size) {
        val from = i.toInt()
        val to = (i + bucket).toInt().coerceAtMost(src.size)
        val slice = src.subList(from, to)
        val inAvg = slice.map { it.trafficInBps ?: 0.0 }.average()
        val outAvg = slice.map { it.trafficOutBps ?: 0.0 }.average()
        out += InterfaceHistoryPoint(timestamp = slice[slice.size / 2].timestamp, trafficInBps = inAvg, trafficOutBps = outAvg)
        i += bucket
    }
    return out
}

fun parseTs(ts: String): OffsetDateTime = try { OffsetDateTime.parse(ts) } catch (_: Exception) { OffsetDateTime.now() }

@Composable
fun MiniLineChart(cpu: List<Double>, mem: List<Double>) {
    val pts = cpu.indices.map { i ->
        InterfaceHistoryPoint(timestamp = i.toString(), trafficInBps = cpu[i], trafficOutBps = mem.getOrElse(i) { 0.0 })
    }
    MpTrafficChart(points = decimateTraffic(pts, 400), modifier = Modifier.fillMaxWidth().height(220.dp))
}

@Composable
fun MiniStatCard(title: String, value: String, brush: Brush, showPulse: Boolean = false) {
    ElevatedCard(
        shape = RoundedCornerShape(Np.corner),
        colors = CardDefaults.elevatedCardColors(containerColor = Color.Transparent),
        modifier = Modifier.width(150.dp)
    ) {
        Box(Modifier.background(brush).padding(12.dp)) {
            Column {
                Text(title, color = Color.White.copy(alpha = 0.9f), style = MaterialTheme.typography.bodySmall)
                Row(verticalAlignment = Alignment.CenterVertically) {
                    if (showPulse) {
                        PulseDot("online")
                        Spacer(Modifier.width(6.dp))
                    }
                    Text(value, color = Color.White, style = MaterialTheme.typography.headlineSmall, fontWeight = FontWeight.Bold)
                }
            }
        }
    }
}

@Composable
fun PulseDot(status: String) {
    val base = if (status == "online") Np.Success else if (status == "offline") Np.Danger else Np.Warning
    val infinite = rememberInfiniteTransition(label = "pulse")
    val alpha by infinite.animateFloat(
        initialValue = 0.4f,
        targetValue = 1f,
        animationSpec = infiniteRepeatable(animation = tween(900), repeatMode = RepeatMode.Reverse),
        label = "pulseAlpha"
    )
    Box(
        Modifier.size(10.dp).clip(CircleShape).background(base.copy(alpha = alpha))
    )
}

@Composable
fun SkeletonBox(modifier: Modifier = Modifier) {
    val infinite = rememberInfiniteTransition(label = "shimmer")
    val x by infinite.animateFloat(
        initialValue = -600f,
        targetValue = 600f,
        animationSpec = infiniteRepeatable(animation = tween(1500, easing = LinearEasing), repeatMode = RepeatMode.Restart),
        label = "goldenShimmerX"
    )
    val brush = Brush.linearGradient(
        colors = listOf(Color(0xFF1E293B), Color(0xFF0F172A), Color(0xFF1E293B)),
        start = Offset(x, 0f),
        end = Offset(x + 420f, 0f)
    )
    Box(modifier.clip(RoundedCornerShape(Np.corner)).background(brush))
}

@Composable
fun SkeletonCard() {
    ElevatedCard(shape = RoundedCornerShape(Np.corner), colors = CardDefaults.elevatedCardColors(containerColor = Color(0xFF1E293B))) {
        Column(Modifier.fillMaxWidth().padding(Np.cardPadding), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            SkeletonBox(Modifier.fillMaxWidth(0.45f).height(14.dp))
            SkeletonBox(Modifier.fillMaxWidth(0.8f).height(12.dp))
            SkeletonBox(Modifier.fillMaxWidth(0.6f).height(12.dp))
        }
    }
}

fun copyToClipboard(context: Context, value: String) {
    if (value.isBlank()) return
    val cm = context.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
    cm.setPrimaryClip(ClipData.newPlainText("ip", value))
}

@Composable
fun EmptyStateCard(title: String, desc: String, modifier: Modifier = Modifier) {
    Card(modifier, shape = RoundedCornerShape(Np.corner), colors = CardDefaults.cardColors(containerColor = Color(0xFF1E293B))) {
        Column(
            Modifier.fillMaxWidth().padding(20.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(6.dp)
        ) {
            Text("◻", style = MaterialTheme.typography.headlineSmall, color = Np.Indigo)
            Text(title, style = MaterialTheme.typography.titleSmall, fontWeight = FontWeight.SemiBold)
            Text(desc, style = MaterialTheme.typography.bodySmall, color = Color.Gray)
        }
    }
}

private fun severityOf(item: AuditLog): String {
    val txt = (item.action + " " + (item.target ?: "")).uppercase()
    return when {
        txt.contains("OFFLINE") || txt.contains("ERROR") || txt.contains("RESTORE") -> "error"
        txt.contains("WARN") || txt.contains("BGP") || txt.contains("OSPF") || txt.contains("FLAP") -> "warning"
        else -> "info"
    }
}

private fun statusText(status: String): String = when (status.lowercase()) {
    "online", "up" -> "在线"
    "offline", "down" -> "离线"
    else -> "未知"
}


@Composable
fun DeviceQuickPeekSheet(vm: MainViewModel, device: DeviceStatus, onOpenDetail: (Long) -> Unit) {
    val cpu by vm.cpu.collectAsStateWithLifecycle()
    val mem by vm.mem.collectAsStateWithLifecycle()
    val loading by vm.loading.collectAsStateWithLifecycle()
    val detail by vm.deviceDetail.collectAsStateWithLifecycle()
    val start = remember { OffsetDateTime.now().minusDays(1) }
    val end = remember { OffsetDateTime.now() }
    LaunchedEffect(device.id) { vm.loadDeviceDetail(device.id, start, end) }

    Column(Modifier.fillMaxWidth().padding(Np.screenPadding), verticalArrangement = Arrangement.spacedBy(Np.sectionGap)) {
        Text("${device.ip} · ${device.brand}", style = MaterialTheme.typography.titleMedium, color = Color.White)
        if (loading) SkeletonBox(Modifier.fillMaxWidth().height(200.dp))
        else MiniLineChart(cpu.map { it.cpuUsage ?: 0.0 }, mem.map { it.memUsage ?: 0.0 })
        Text("端口", color = Color.White, style = MaterialTheme.typography.titleSmall)
        LazyColumn(Modifier.heightIn(max = 220.dp)) {
            items(detail?.interfaces.orEmpty(), key = { it.id }) { p ->
                Text("${p.name} (#${p.index})", color = Color.White, modifier = Modifier.padding(vertical = 4.dp))
            }
        }
        Button(onClick = { onOpenDetail(device.id); vm.closeQuickPeek() }, modifier = Modifier.fillMaxWidth()) { Text("进入详情") }
    }
}
