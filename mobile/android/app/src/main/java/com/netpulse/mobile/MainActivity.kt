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
import androidx.compose.animation.AnimatedContentTransitionScope
import androidx.compose.animation.core.tween
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.clickable
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.ExperimentalMaterialApi
import androidx.compose.material.pullrefresh.PullRefreshIndicator
import androidx.compose.material.pullrefresh.pullRefresh
import androidx.compose.material.pullrefresh.rememberPullRefreshState
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
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

private object UiSpec {
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
            MaterialTheme {
                NetPulseApp(vm = vm, onBiometricLogin = { triggerBiometricLogin() })
            }
        }
    }

    private fun triggerBiometricLogin() {
        val biometricManager = BiometricManager.from(this)
        if (biometricManager.canAuthenticate(BiometricManager.Authenticators.BIOMETRIC_STRONG) != BiometricManager.BIOMETRIC_SUCCESS) return
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

@Composable
fun NetPulseApp(vm: MainViewModel, onBiometricLogin: () -> Unit) {
    val nav = rememberNavController()
    val token by vm.token.collectAsStateWithLifecycle()
    val devices by vm.devices.collectAsStateWithLifecycle()
    val msg by vm.message.collectAsStateWithLifecycle()
    val loading by vm.loading.collectAsStateWithLifecycle()

    LaunchedEffect(token) { if (token.isNotBlank()) vm.refreshDevices() }
    val snackState = remember { SnackbarHostState() }
    LaunchedEffect(msg) { if (msg.isNotBlank()) snackState.showSnackbar(msg) }
    LaunchedEffect(token) {
        if (token.isBlank()) nav.navigate("login") { popUpTo(0) }
        else if (nav.currentDestination?.route == "login") nav.navigate("home") { popUpTo("login") { inclusive = true } }
    }

    Scaffold(snackbarHost = { SnackbarHost(hostState = snackState) }) {
        NavHost(navController = nav, startDestination = if (token.isBlank()) "login" else "home") {
            composable("login") {
                LoginScreen(
                    loading = loading,
                    onLogin = { u, p -> vm.login(u, p) },
                    onBio = onBiometricLogin,
                    onSaveBase = { vm.saveBaseUrl(it) },
                    hint = "默认地址: http://119.40.55.18:18080/api"
                )
            }
            composable(
                route = "home",
                enterTransition = { slideIntoContainer(AnimatedContentTransitionScope.SlideDirection.Left, tween(220)) },
                exitTransition = { slideOutOfContainer(AnimatedContentTransitionScope.SlideDirection.Left, tween(220)) }
            ) {
                HomeScreen(devices, loading, vm::refreshDevices, { id -> nav.navigate("device/$id") }, vm::logout)
            }
            composable(
                route = "device/{id}",
                arguments = listOf(navArgument("id") { type = NavType.LongType }),
                enterTransition = { slideIntoContainer(AnimatedContentTransitionScope.SlideDirection.Left, tween(220)) },
                popExitTransition = { slideOutOfContainer(AnimatedContentTransitionScope.SlideDirection.Right, tween(220)) }
            ) { backStack ->
                val id = backStack.arguments?.getLong("id") ?: 0L
                DeviceDetailScreen(id, vm, onBack = { nav.popBackStack() }, onOpenPort = { portId -> nav.navigate("port/$portId") })
            }
            composable(
                route = "port/{id}",
                arguments = listOf(navArgument("id") { type = NavType.LongType }),
                enterTransition = { slideIntoContainer(AnimatedContentTransitionScope.SlideDirection.Left, tween(220)) },
                popExitTransition = { slideOutOfContainer(AnimatedContentTransitionScope.SlideDirection.Right, tween(220)) }
            ) { backStack ->
                val id = backStack.arguments?.getLong("id") ?: 0L
                PortDetailScreen(id, vm, onBack = { nav.popBackStack() })
            }
        }
    }
}

@Composable
fun LoginScreen(loading: Boolean, onLogin: (String, String) -> Unit, onBio: () -> Unit, onSaveBase: (String) -> Unit, hint: String) {
    var u by remember { mutableStateOf("") }
    var p by remember { mutableStateOf("") }
    var base by remember { mutableStateOf("http://119.40.55.18:18080/api") }
    Column(Modifier.fillMaxSize().padding(UiSpec.screenPadding), verticalArrangement = Arrangement.Center) {
        Text("NetPulse 移动端", style = MaterialTheme.typography.headlineSmall, fontWeight = FontWeight.Bold)
        Spacer(Modifier.height(UiSpec.sectionGap))
        OutlinedTextField(u, { u = it }, label = { Text("用户名") }, modifier = Modifier.fillMaxWidth())
        Spacer(Modifier.height(8.dp))
        OutlinedTextField(p, { p = it }, label = { Text("密码") }, modifier = Modifier.fillMaxWidth())
        Spacer(Modifier.height(8.dp))
        OutlinedTextField(base, { base = it }, label = { Text("服务器 API 地址") }, modifier = Modifier.fillMaxWidth())
        Text(hint, style = MaterialTheme.typography.bodySmall, color = Color.Gray)
        Spacer(Modifier.height(UiSpec.sectionGap))
        Button(onClick = { onSaveBase(base); onLogin(u, p) }, modifier = Modifier.fillMaxWidth(), enabled = !loading) { Text("登录") }
        Spacer(Modifier.height(8.dp))
        OutlinedButton(onClick = onBio, modifier = Modifier.fillMaxWidth()) { Text("生物识别快速登录") }
        Text("首次登录必须输入用户名密码", style = MaterialTheme.typography.bodySmall, color = Color.Gray)
    }
}

@OptIn(ExperimentalFoundationApi::class, ExperimentalMaterial3Api::class)
@Composable
fun HomeScreen(devices: List<DeviceStatus>, loading: Boolean, onRefresh: () -> Unit, onOpen: (Long) -> Unit, onLogout: () -> Unit) {
    val total = devices.size
    val online = devices.count { it.status == "online" }
    val offline = devices.count { it.status == "offline" }
    val unknown = total - online - offline
    val ctx = LocalContext.current

    Scaffold(topBar = {
        TopAppBar(title = { Text("资产总览") }, actions = {
            TextButton(onClick = onRefresh) { Text("刷新") }
            TextButton(onClick = onLogout) { Text("退出") }
        })
    }) { p ->
        Column(Modifier.padding(p).fillMaxSize().padding(UiSpec.screenPadding), verticalArrangement = Arrangement.spacedBy(UiSpec.sectionGap)) {
            Surface(tonalElevation = 2.dp, shape = MaterialTheme.shapes.medium) {
                Row(Modifier.fillMaxWidth().padding(UiSpec.cardPadding), horizontalArrangement = Arrangement.SpaceBetween) {
                    Text("总数 $total")
                    Text("在线 $online", color = Color(0xFF2E7D32))
                    Text("离线 $offline", color = Color(0xFFC62828))
                    Text("未知 $unknown", color = Color(0xFFD97706))
                }
            }
            if (loading) LinearProgressIndicator(Modifier.fillMaxWidth())
            if (!loading && devices.isEmpty()) {
                EmptyStateCard("暂无资产", "请先在 Web 端创建普通用户并添加设备")
            }
            LazyColumn(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                items(devices, key = { it.id }) { d ->
                    ElevatedCard(modifier = Modifier.fillMaxWidth().clickable { onOpen(d.id) }, shape = RoundedCornerShape(UiSpec.corner)) {
                        Column(Modifier.padding(UiSpec.cardPadding)) {
                            Row(verticalAlignment = Alignment.CenterVertically) {
                                Box(Modifier.size(10.dp)) {
                                    Surface(
                                        color = when (d.status) {
                                            "online" -> Color(0xFF2E7D32)
                                            "offline" -> Color(0xFFC62828)
                                            else -> Color(0xFFD97706)
                                        },
                                        shape = MaterialTheme.shapes.small,
                                        modifier = Modifier.fillMaxSize()
                                    ) {}
                                }
                                Spacer(Modifier.width(8.dp))
                                Text(
                                    d.ip,
                                    style = MaterialTheme.typography.titleMedium,
                                    modifier = Modifier.combinedClickable(
                                        onClick = {},
                                        onLongClick = { copyToClipboard(ctx, d.ip) }
                                    )
                                )
                            }
                            Text("${d.brand} · ${d.remark.ifBlank { "未备注" }}", style = MaterialTheme.typography.bodyMedium)
                            if (!d.statusReason.isNullOrBlank()) {
                                Text(d.statusReason!!, style = MaterialTheme.typography.bodySmall, color = Color.Gray)
                            }
                        }
                    }
                }
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class, ExperimentalMaterialApi::class, ExperimentalFoundationApi::class)
@Composable
fun DeviceDetailScreen(deviceId: Long, vm: MainViewModel, onBack: () -> Unit, onOpenPort: (Long) -> Unit) {
    val device by vm.deviceDetail.collectAsStateWithLifecycle()
    val cpu by vm.cpu.collectAsStateWithLifecycle()
    val mem by vm.mem.collectAsStateWithLifecycle()
    val logs by vm.logs.collectAsStateWithLifecycle()
    val loading by vm.loading.collectAsStateWithLifecycle()
    var keyword by remember { mutableStateOf("") }
    var start by remember { mutableStateOf(OffsetDateTime.now().minusDays(1)) }
    var end by remember { mutableStateOf(OffsetDateTime.now()) }
    var editingPort by remember { mutableStateOf<NetInterface?>(null) }
    var portRemark by remember { mutableStateOf("") }
    val ctx = LocalContext.current

    LaunchedEffect(deviceId) { vm.loadDeviceDetail(deviceId, start, end) }

    val ports = device?.interfaces.orEmpty().filter {
        val k = keyword.lowercase().trim()
        if (k.isBlank()) true else "${it.id} ${it.index} ${it.name} ${it.remark}".lowercase().contains(k)
    }

    val refreshState = rememberPullRefreshState(loading, { vm.loadDeviceDetail(deviceId, start, end) })

    Scaffold(topBar = {
        TopAppBar(
            title = { Text("设备详情") },
            navigationIcon = { IconButton(onClick = onBack) { Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = null) } }
        )
    }) { p ->
        Box(Modifier.padding(p).fillMaxSize().pullRefresh(refreshState)) {
            LazyColumn(Modifier.fillMaxSize().padding(UiSpec.screenPadding), verticalArrangement = Arrangement.spacedBy(UiSpec.sectionGap)) {
                item {
                    ElevatedCard(Modifier.fillMaxWidth()) {
                        Column(Modifier.padding(UiSpec.cardPadding)) {
                            Text(
                                device?.ip ?: "-",
                                style = MaterialTheme.typography.titleMedium,
                                modifier = Modifier.combinedClickable(onClick = {}, onLongClick = { copyToClipboard(ctx, device?.ip ?: "") })
                            )
                            Text("${device?.brand ?: "-"} · ${device?.remark ?: "-"}")
                            if (!device?.statusReason.isNullOrBlank()) {
                                Text("状态说明：${device?.statusReason}", style = MaterialTheme.typography.bodySmall, color = Color.Gray)
                            }
                        }
                    }
                }
                item {
                    ElevatedCard(Modifier.fillMaxWidth()) {
                        Column(Modifier.padding(UiSpec.cardPadding)) {
                            Text("CPU / 内存", style = MaterialTheme.typography.titleMedium)
                            Spacer(Modifier.height(8.dp))
                            MiniLineChart(cpu.map { it.cpuUsage ?: 0.0 }, mem.map { it.memUsage ?: 0.0 })
                        }
                    }
                }
                item {
                    OutlinedTextField(keyword, { keyword = it }, label = { Text("搜索端口 id/索引/名称/备注") }, modifier = Modifier.fillMaxWidth())
                }
                if (ports.isEmpty()) {
                    item { EmptyStateCard("暂无端口", "SNMP 同步成功后会显示端口列表") }
                }
                items(ports, key = { it.id }) { itf ->
                    ElevatedCard(
                        Modifier.fillMaxWidth().combinedClickable(
                            onClick = { onOpenPort(itf.id) },
                            onLongClick = {
                                editingPort = itf
                                portRemark = itf.remark
                            }
                        )
                    ) {
                        Column(Modifier.padding(UiSpec.cardPadding)) {
                            Text(itf.name, fontWeight = FontWeight.SemiBold)
                            Text("索引: ${itf.index} · 备注: ${itf.remark.ifBlank { "-" }}")
                            Text("点击看流量，长按改备注", color = Color.Gray)
                        }
                    }
                }
                item {
                    ElevatedCard(Modifier.fillMaxWidth()) {
                        Column(Modifier.padding(UiSpec.cardPadding), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                            Text("最近日志", style = MaterialTheme.typography.titleMedium)
                            if (logs.isEmpty()) {
                                Text("暂无日志", color = Color.Gray, style = MaterialTheme.typography.bodySmall)
                            } else {
                                logs.take(100).forEach { log ->
                                    val c = when (log.level.uppercase()) {
                                        "ERROR" -> Color(0xFFC62828)
                                        "WARNING", "WARN" -> Color(0xFFEF6C00)
                                        else -> Color(0xFF2E7D32)
                                    }
                                    Text(
                                        "[${log.level}] ${log.message}",
                                        color = c,
                                        style = MaterialTheme.typography.bodySmall
                                    )
                                }
                            }
                        }
                    }
                }
            }
            PullRefreshIndicator(loading, refreshState, Modifier.align(Alignment.TopCenter))
        }
    }

    if (editingPort != null) {
        AlertDialog(
            onDismissRequest = { editingPort = null },
            title = { Text("编辑端口备注") },
            text = {
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Text(editingPort?.name ?: "")
                    OutlinedTextField(
                        value = portRemark,
                        onValueChange = { portRemark = it },
                        label = { Text("备注") },
                        modifier = Modifier.fillMaxWidth()
                    )
                }
            },
            confirmButton = {
                TextButton(onClick = {
                    val target = editingPort ?: return@TextButton
                    vm.updateInterfaceRemark(deviceId, target.id, portRemark.trim(), start, end)
                    editingPort = null
                }) { Text("保存") }
            },
            dismissButton = {
                TextButton(onClick = { editingPort = null }) { Text("取消") }
            }
        )
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PortDetailScreen(portId: Long, vm: MainViewModel, onBack: () -> Unit) {
    val traffic by vm.traffic.collectAsStateWithLifecycle()
    var start by remember { mutableStateOf(OffsetDateTime.now().minusDays(1)) }
    var end by remember { mutableStateOf(OffsetDateTime.now()) }
    val nowMs = System.currentTimeMillis()
    val minMs = nowMs - 3L * 365 * 24 * 3600 * 1000
    var showStartPicker by remember { mutableStateOf(false) }
    var showEndPicker by remember { mutableStateOf(false) }

    LaunchedEffect(portId) { vm.loadPortTraffic(portId, start, end) }

    Scaffold(topBar = {
        TopAppBar(
            title = { Text("端口流量") },
            navigationIcon = { IconButton(onClick = onBack) { Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = null) } },
            actions = { TextButton(onClick = { vm.loadPortTraffic(portId, start, end) }) { Text("刷新") } }
        )
    }) { p ->
        Column(Modifier.padding(p).fillMaxSize().padding(UiSpec.screenPadding), verticalArrangement = Arrangement.spacedBy(UiSpec.sectionGap)) {
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                OutlinedButton(onClick = { showStartPicker = true }) { Text("开始: ${start.toLocalDate()}") }
                OutlinedButton(onClick = { showEndPicker = true }) { Text("结束: ${end.toLocalDate()}") }
            }
            Text("支持 3 年查询范围", color = Color.Gray, style = MaterialTheme.typography.bodySmall)
            Surface(tonalElevation = 2.dp, shape = MaterialTheme.shapes.medium, modifier = Modifier.fillMaxWidth().weight(1f)) {
                if (traffic.isEmpty()) {
                    EmptyStateCard("暂无流量数据", "请调整时间范围后刷新", modifier = Modifier.fillMaxSize())
                } else {
                    val decimated = remember(traffic) { decimateTraffic(traffic, 1800) }
                    MpTrafficChart(points = decimated, modifier = Modifier.fillMaxSize().padding(10.dp))
                }
            }
        }
    }

    if (showStartPicker) {
        val state = rememberDatePickerState(initialSelectedDateMillis = start.toInstant().toEpochMilli(), yearRange = IntRange(OffsetDateTime.now().year - 3, OffsetDateTime.now().year))
        DatePickerDialog(onDismissRequest = { showStartPicker = false }, confirmButton = {
            TextButton(onClick = {
                state.selectedDateMillis?.let {
                    val clamped = it.coerceIn(minMs, nowMs)
                    start = Date(clamped).toInstant().atZone(ZoneId.systemDefault()).toOffsetDateTime()
                }
                showStartPicker = false
            }) { Text("确定") }
        }) { DatePicker(state = state) }
    }
    if (showEndPicker) {
        val state = rememberDatePickerState(initialSelectedDateMillis = end.toInstant().toEpochMilli(), yearRange = IntRange(OffsetDateTime.now().year - 3, OffsetDateTime.now().year))
        DatePickerDialog(onDismissRequest = { showEndPicker = false }, confirmButton = {
            TextButton(onClick = {
                state.selectedDateMillis?.let {
                    val clamped = it.coerceIn(minMs, nowMs)
                    end = Date(clamped).toInstant().atZone(ZoneId.systemDefault()).toOffsetDateTime()
                }
                showEndPicker = false
            }) { Text("确定") }
        }) { DatePicker(state = state) }
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
                    val dt = parseTs(points[i].timestamp)
                    return dt.format(formatter)
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
            color = android.graphics.Color.parseColor("#2E7D32")
            setDrawCircles(false)
            lineWidth = 1.8f
            mode = LineDataSet.Mode.LINEAR
        }
        val outSet = LineDataSet(outEntries, "出方向").apply {
            color = android.graphics.Color.parseColor("#EF6C00")
            setDrawCircles(false)
            lineWidth = 1.8f
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

fun parseTs(ts: String): OffsetDateTime {
    return try { OffsetDateTime.parse(ts) } catch (_: Exception) { OffsetDateTime.now() }
}

@Composable
fun MiniLineChart(cpu: List<Double>, mem: List<Double>) {
    val pts = cpu.indices.map { i -> InterfaceHistoryPoint(timestamp = i.toString(), trafficInBps = cpu[i], trafficOutBps = mem.getOrElse(i) { 0.0 }) }
    MpTrafficChart(points = decimateTraffic(pts, 400), modifier = Modifier.fillMaxWidth().height(220.dp))
}

fun copyToClipboard(context: Context, value: String) {
    if (value.isBlank()) return
    val cm = context.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
    cm.setPrimaryClip(ClipData.newPlainText("ip", value))
}

@Composable
fun EmptyStateCard(title: String, desc: String, modifier: Modifier = Modifier) {
    Card(modifier, shape = RoundedCornerShape(UiSpec.corner), colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceVariant.copy(alpha = 0.35f))) {
        Column(
            Modifier.fillMaxWidth().padding(20.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(6.dp)
        ) {
            Text("□", style = MaterialTheme.typography.headlineSmall, color = MaterialTheme.colorScheme.primary)
            Text(title, style = MaterialTheme.typography.titleSmall, fontWeight = FontWeight.SemiBold)
            Text(desc, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
}
