package com.netpulse.mobile

import android.content.ContentValues
import android.content.Context
import android.graphics.Bitmap
import android.graphics.Canvas as AndroidCanvas
import android.graphics.Color as AndroidColor
import android.graphics.Paint
import android.graphics.Path as AndroidPath
import android.os.Build
import android.os.Bundle
import android.provider.MediaStore
import android.widget.Toast
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.detectDragGestures
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.gestures.detectTransformGestures
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.toArgb
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.navigation.NavType
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.rememberNavController
import androidx.navigation.navArgument
import com.netpulse.mobile.data.*
import com.netpulse.mobile.util.displayPortName
import com.netpulse.mobile.util.formatBps
import com.netpulse.mobile.util.formatTime
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.time.Instant
import java.time.LocalDateTime
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.temporal.ChronoUnit
import kotlin.math.ceil
import kotlin.math.abs
import kotlin.math.max
import kotlin.math.roundToInt

private val Navy = Color(0xFF0F172A)
private val Card = Color(0xFF1E293B)
private val Text = Color(0xFFF8FAFC)
private val Muted = Color(0xFFB7C3D6)
private val Indigo = Color(0xFF6366F1)
private val Green = Color(0xFF22C55E)
private val Red = Color(0xFFEF4444)
private val Orange = Color(0xFFF59E0B)
private val Cyan = Color(0xFF06B6D4)
private val ServerZone: ZoneId = ZoneId.of("Asia/Shanghai")

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val session = SessionStore(this)
        setContent { NetPulseApp(session) }
    }
}

@Composable
fun NetPulseApp(session: SessionStore) {
    val api = remember(session.baseUrl, session.token) { ApiClient(session) }
    val nav = rememberNavController()
    var authed by remember { mutableStateOf(session.token.isNotBlank()) }

    MaterialTheme(colorScheme = darkColorScheme(primary = Indigo, background = Navy, surface = Card)) {
        Surface(Modifier.fillMaxSize(), color = Navy) {
            if (!authed) {
                LoginScreen(session) { authed = true }
            } else {
                NavHost(navController = nav, startDestination = "home") {
                    composable("home") { HomeScreen(api, onLogout = { session.clear(); authed = false }, openDevice = { nav.navigate("device/$it") }, openPort = { nav.navigate("port/$it") }) }
                    composable("device/{id}", arguments = listOf(navArgument("id") { type = NavType.LongType })) { entry ->
                        DeviceDetailScreen(api, entry.arguments?.getLong("id") ?: 0L, back = { nav.popBackStack() }, openPort = { nav.navigate("port/$it") })
                    }
                    composable("port/{id}", arguments = listOf(navArgument("id") { type = NavType.LongType })) { entry ->
                        PortDetailScreen(api, entry.arguments?.getLong("id") ?: 0L, back = { nav.popBackStack() })
                    }
                }
            }
        }
    }
}

@Composable
fun LoginScreen(session: SessionStore, onSuccess: () -> Unit) {
    val scope = rememberCoroutineScope()
    var username by remember { mutableStateOf("") }
    var password by remember { mutableStateOf("") }
    var baseUrl by remember { mutableStateOf(session.baseUrl) }
    var loading by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf("") }

    Column(
        Modifier.fillMaxSize().padding(28.dp),
        verticalArrangement = Arrangement.Center,
        horizontalAlignment = Alignment.CenterHorizontally
    ) {
        Text("NetPulse", color = Text, fontSize = 38.sp, fontWeight = FontWeight.Bold)
        Text("Android 只读工作台", color = Muted, modifier = Modifier.padding(top = 8.dp, bottom = 28.dp))
        NPTextField(username, { username = it }, "用户名")
        Spacer(Modifier.height(10.dp))
        NPTextField(password, { password = it }, "密码", password = true)
        Spacer(Modifier.height(10.dp))
        NPTextField(baseUrl, { baseUrl = it }, "后端地址，例如 http://server:8080/api")
        Spacer(Modifier.height(20.dp))
        NPButton(enabled = !loading, onClick = {
            loading = true
            error = ""
            session.baseUrl = baseUrl
            scope.launch {
                runCatching { withContext(Dispatchers.IO) { ApiClient(session).login(username, password) } }
                    .onSuccess { onSuccess() }
                    .onFailure { error = "登录失败：${it.message ?: "请检查账号密码与地址"}" }
                loading = false
            }
        }) { Text(if (loading) "登录中..." else "登录") }
        if (error.isNotBlank()) Text(error, color = Red, modifier = Modifier.padding(top = 16.dp))
    }
}

@Composable
fun HomeScreen(api: ApiClient, onLogout: () -> Unit, openDevice: (Long) -> Unit, openPort: (Long) -> Unit) {
    val scope = rememberCoroutineScope()
    val focus = LocalFocusManager.current
    var devices by remember { mutableStateOf<List<Device>>(emptyList()) }
    var search by remember { mutableStateOf("") }
    var results by remember { mutableStateOf<List<SearchItem>>(emptyList()) }
    var loading by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf("") }

    fun refresh() {
        loading = true
        error = ""
        scope.launch {
            runCatching { withContext(Dispatchers.IO) { api.fetchDevices() } }
                .onSuccess { devices = it.sortedWith(compareBy<Device> { tierRank(it) }.thenBy { it.name.ifBlank { it.ip } }) }
                .onFailure { error = it.message ?: "加载资产失败" }
            loading = false
        }
    }
    LaunchedEffect(Unit) { refresh() }
    LaunchedEffect(search) {
        val q = search.trim()
        results = if (q.length >= 2) runCatching { withContext(Dispatchers.IO) { api.search(q) } }.getOrDefault(emptyList()) else emptyList()
    }

    LazyColumn(Modifier.fillMaxSize().padding(18.dp), contentPadding = PaddingValues(bottom = 28.dp)) {
        item {
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween, verticalAlignment = Alignment.CenterVertically) {
                Text("资产中心", color = Text, fontSize = 32.sp, fontWeight = FontWeight.Bold)
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    NPOutlinedButton(onClick = { refresh() }) { Text("刷新") }
                    NPOutlinedButton(onClick = onLogout) { Text("退出") }
                }
            }
            Spacer(Modifier.height(12.dp))
            NPTextField(search, { search = it }, "搜索资产 / IP / 端口名称", onDone = { focus.clearFocus() })
            Spacer(Modifier.height(12.dp))
            StatsRow(devices)
            if (error.isNotBlank()) Text(error, color = Red, modifier = Modifier.padding(vertical = 10.dp))
            if (results.isNotEmpty()) SearchResults(results, openDevice, openPort) { focus.clearFocus() }
            Text(if (loading) "加载中..." else "资产列表", color = Muted, modifier = Modifier.padding(vertical = 12.dp))
        }
        items(devices, key = { it.id }) { d -> DeviceRow(d, openDevice) }
    }
}

@Composable
fun DeviceDetailScreen(api: ApiClient, id: Long, back: () -> Unit, openPort: (Long) -> Unit) {
    var device by remember { mutableStateOf<Device?>(null) }
    var cpuMem by remember { mutableStateOf<List<DeviceHistoryPoint>>(emptyList()) }
    var showCpu by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf("") }

    LaunchedEffect(id) {
        runCatching { withContext(Dispatchers.IO) { api.fetchDevice(id) } }
            .onSuccess { device = it }
            .onFailure { error = it.message ?: "加载设备详情失败" }
        runCatching {
            val end = Instant.now(); val start = end.minus(24, ChronoUnit.HOURS)
            withContext(Dispatchers.IO) { api.fetchDeviceHistory("cpu", id, start, end, 720, "2m") }
        }.onSuccess { cpuMem = it }
    }

    LazyColumn(Modifier.fillMaxSize().padding(18.dp), contentPadding = PaddingValues(bottom = 28.dp)) {
        item {
            BackTitle("设备详情", back)
            if (error.isNotBlank()) Text(error, color = Red)
            device?.let { d ->
                CardBlock {
                    Row(verticalAlignment = Alignment.CenterVertically) { StatusDot(d.status); Spacer(Modifier.width(10.dp)); Text(d.name.ifBlank { d.ip }, color = Text, fontSize = 24.sp, fontWeight = FontWeight.Bold) }
                    Text("${d.statusLabel()} · ${d.ip} · ${d.brand} · ${d.remark.ifBlank { "未备注" }}", color = Muted, modifier = Modifier.padding(top = 8.dp))
                }
                CardBlock {
                    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
                        Text("CPU / 内存", color = Text, fontWeight = FontWeight.Bold)
                        NPOutlinedButton(onClick = { showCpu = !showCpu }) { Text(if (showCpu) "隐藏" else "展开") }
                    }
                    if (showCpu) CpuMemChart(cpuMem)
                }
                Text("端口列表", color = Muted, modifier = Modifier.padding(vertical = 12.dp))
            }
        }
        items(device?.interfaces.orEmpty(), key = { it.id }) { p -> PortRow(p, openPort) }
    }
}

@Composable
fun PortDetailScreen(api: ApiClient, id: Long, back: () -> Unit) {
    val scope = rememberCoroutineScope()
    val context = LocalContext.current
    var port by remember { mutableStateOf<Port?>(null) }
    var points by remember { mutableStateOf<List<TrafficHistoryPoint>>(emptyList()) }
    var range by remember { mutableStateOf("day") }
    var loading by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf("") }
    var showCustom by remember { mutableStateOf(false) }
    var customStart by remember { mutableStateOf(defaultCustomStartText()) }
    var customEnd by remember { mutableStateOf(defaultCustomEndText()) }
    var readoutEnabled by remember { mutableStateOf(false) }
    var selectedIndex by remember { mutableStateOf<Int?>(null) }

    fun load(r: String, custom: Pair<Instant, Instant>? = null) {
        loading = true
        error = ""
        range = r
        selectedIndex = null
        scope.launch {
            runCatching {
                val spec = rangeSpec(r, custom)
                withContext(Dispatchers.IO) { api.fetchTraffic(id, spec.start, spec.end, spec.maxPoints, spec.interval) }
            }.onSuccess { points = it }.onFailure { error = it.message ?: "加载流量失败" }
            loading = false
        }
    }

    LaunchedEffect(id) {
        val fetched = runCatching { withContext(Dispatchers.IO) { api.fetchPort(id) } }.getOrNull()
        port = fetched
        load("day")
    }

    LazyColumn(Modifier.fillMaxSize().padding(18.dp), contentPadding = PaddingValues(bottom = 28.dp)) {
        item {
            BackTitle("端口详情", back)
            port?.let { p ->
                CardBlock {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        StatusDot(if (p.operStatus == 1) "up" else "down", p.adminStatus)
                        Spacer(Modifier.width(10.dp))
                        Text(displayPortName(p.name, p.customName, p.remark), color = Text, fontSize = 24.sp, fontWeight = FontWeight.Bold)
                    }
                    Text("索引:${p.index} · ${p.speedMbps ?: 0} Mbps · ${p.remark.ifBlank { "未备注" }}", color = Muted, modifier = Modifier.padding(top = 6.dp))
                }
            }
            RangeButtons(range, ::load)
            CardBlock {
                Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween, verticalAlignment = Alignment.CenterVertically) {
                    Text("自定义周期查询", color = Text, fontWeight = FontWeight.Bold)
                    NPOutlinedButton(onClick = { showCustom = !showCustom }) { Text(if (showCustom) "隐藏" else "展开") }
                }
                if (showCustom) {
                    Text("格式：yyyy-MM-dd HH:mm，按服务器时区 Asia/Shanghai 查询", color = Muted, fontSize = 12.sp)
                    Spacer(Modifier.height(8.dp))
                    NPTextField(customStart, { customStart = it }, "开始时间")
                    Spacer(Modifier.height(8.dp))
                    NPTextField(customEnd, { customEnd = it }, "结束时间")
                    Spacer(Modifier.height(8.dp))
                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        NPButton(onClick = {
                            runCatching { parseCustomRange(customStart, customEnd) }
                                .onSuccess { load("custom", it) }
                                .onFailure { error = it.message ?: "时间格式错误" }
                        }) { Text("查询") }
                        NPOutlinedButton(onClick = { customStart = defaultCustomStartText(); customEnd = defaultCustomEndText() }) { Text("重置") }
                    }
                }
            }
            CardBlock {
                Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween, verticalAlignment = Alignment.CenterVertically) {
                    Text(if (loading) "流量加载中..." else "端口流量", color = Text, fontWeight = FontWeight.Bold)
                    NPOutlinedButton(onClick = {
                        val title = port?.let { displayPortName(it.name, it.customName, it.remark) } ?: "端口$id"
                        val ok = saveTrafficChart(context, points, title, rangeLabel(range), selectedIndex)
                        Toast.makeText(context, if (ok) "图表已保存到相册" else "图表保存失败", Toast.LENGTH_SHORT).show()
                    }) { Text("保存图表") }
                }
                if (error.isNotBlank()) Text(error, color = Red)
                TrafficChart(
                    points = points,
                    label = rangeLabel(range),
                    readoutEnabled = readoutEnabled,
                    selectedIndex = selectedIndex,
                    onReadoutToggle = { readoutEnabled = !readoutEnabled },
                    onSelected = { selectedIndex = it }
                )
            }
        }
    }
}

@Composable
fun TrafficChart(
    points: List<TrafficHistoryPoint>,
    label: String = "",
    readoutEnabled: Boolean = false,
    selectedIndex: Int? = null,
    onReadoutToggle: () -> Unit = {},
    onSelected: (Int?) -> Unit = {}
) {
    val scroll = rememberScrollState()
    var zoom by remember { mutableFloatStateOf(1.0f) }
    val normalizedZoom = zoom.coerceIn(0.45f, 4.0f)
    val width = (520.dp * normalizedZoom)
    val values = points.flatMap { listOfNotNull(it.trafficInBps, it.trafficOutBps) }.filter { !it.isNaN() }
    val safeSelected = selectedIndex?.takeIf { it in points.indices && hasTrafficValue(points[it]) }
    val selectedPoint = safeSelected?.let { points[it] }
    Column {
        Row(Modifier.padding(vertical = 8.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            Legend("入方向", Indigo)
            Legend("出方向", Green)
            if (label.isNotBlank()) Text(label, color = Muted, fontSize = 12.sp)
        }
        Row(
            Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()),
            horizontalArrangement = Arrangement.spacedBy(8.dp),
            verticalAlignment = Alignment.CenterVertically
        ) {
            NPOutlinedButton(onClick = { zoom = (zoom / 1.25f).coerceIn(0.45f, 4.0f) }) { Text("缩小") }
            NPOutlinedButton(onClick = { zoom = (zoom * 1.25f).coerceIn(0.45f, 4.0f) }) { Text("放大") }
            NPOutlinedButton(onClick = { zoom = 1.0f; onSelected(null) }) { Text("重置") }
            NPOutlinedButton(onClick = onReadoutToggle) { Text(if (readoutEnabled) "关闭读数" else "开启读数") }
        }
        if (readoutEnabled) {
            Text("读数模式下可点按/拖动查看数值；关闭读数后可横向滑动图表。", color = Muted, fontSize = 11.sp, modifier = Modifier.padding(top = 6.dp))
        }
        selectedPoint?.let {
            Text(
                "读数 ${formatTime(it.timestamp)}  入:${formatReadoutBps(it.trafficInBps)}  出:${formatReadoutBps(it.trafficOutBps)}",
                color = Text,
                fontSize = 12.sp,
                modifier = Modifier.padding(top = 6.dp)
            )
        }
        if (points.isEmpty() || values.isEmpty()) {
            Box(Modifier.fillMaxWidth().height(220.dp), contentAlignment = Alignment.Center) {
                Text("该时间段暂无流量曲线", color = Muted)
            }
        } else {
            Box(Modifier.fillMaxWidth().height(260.dp).horizontalScroll(scroll)) {
                Canvas(Modifier.width(width).fillMaxHeight()
                    .pointerInput(readoutEnabled, points.size) {
                        if (readoutEnabled) {
                            detectTapGestures { offset ->
                                onSelected(nearestTrafficIndex(points, offset.x, size.width.toFloat(), 54f, 18f))
                            }
                        }
                    }
                    .pointerInput(readoutEnabled, points.size) {
                        if (readoutEnabled) {
                            detectDragGestures { change, _ ->
                                onSelected(nearestTrafficIndex(points, change.position.x, size.width.toFloat(), 54f, 18f))
                            }
                        }
                    }
                    .pointerInput(Unit) {
                        detectTransformGestures { _, _, z, _ -> zoom = (zoom * z).coerceIn(0.45f, 4.0f) }
                    }
                ) {
                    val padL = 54f; val padR = 18f; val padT = 18f; val padB = 42f
                    val w = size.width - padL - padR; val h = size.height - padT - padB
                    val maxY = niceMax(values.maxOrNull() ?: 1.0)
                    repeat(5) { i ->
                        val y = padT + h * i / 4f
                        drawLine(Color(0x33475569), Offset(padL, y), Offset(padL + w, y), strokeWidth = 1f)
                    }
                    fun pathOf(vs: List<Double?>): Path {
                        val p = Path(); var started = false
                        vs.forEachIndexed { i, v ->
                        if (v == null || v.isNaN()) return@forEachIndexed
                            val x = padL + if (points.size <= 1) 0f else w * i / (points.size - 1).toFloat()
                            val y = padT + h - (h * (v / maxY).toFloat()).coerceIn(0f, h)
                            if (!started) { p.moveTo(x, y); started = true } else p.lineTo(x, y)
                        }
                        return p
                    }
                    drawPath(pathOf(points.map { it.trafficInBps }), Indigo, style = Stroke(width = 4f, cap = StrokeCap.Round))
                    drawPath(pathOf(points.map { it.trafficOutBps }), Green, style = Stroke(width = 4f, cap = StrokeCap.Round))
                    safeSelected?.let { idx ->
                        val x = padL + if (points.size <= 1) 0f else w * idx / (points.size - 1).toFloat()
                        drawLine(Color(0x99E2E8F0), Offset(x, padT), Offset(x, padT + h), strokeWidth = 2f)
                        fun drawPoint(v: Double?, c: Color) {
                            if (v == null || v.isNaN()) return
                            val y = padT + h - (h * (v / maxY).toFloat()).coerceIn(0f, h)
                            drawCircle(c, radius = 6f, center = Offset(x, y))
                            drawCircle(Color(0xFF0F172A), radius = 3f, center = Offset(x, y))
                        }
                        drawPoint(points[idx].trafficInBps, Indigo)
                        drawPoint(points[idx].trafficOutBps, Green)
                    }
                }
            }
        }
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
            Text(formatBps(0.0), color = Muted, fontSize = 11.sp)
            points.firstOrNull()?.let { Text(formatTime(it.timestamp), color = Muted, fontSize = 11.sp) }
            points.lastOrNull()?.let { Text(formatTime(it.timestamp), color = Muted, fontSize = 11.sp) }
        }
    }
}

@Composable
fun CpuMemChart(points: List<DeviceHistoryPoint>) {
    CardBlock {
        Row { Legend("CPU", Orange); Spacer(Modifier.width(16.dp)); Legend("内存", Cyan) }
        Canvas(Modifier.fillMaxWidth().height(180.dp)) {
            val pad = 24f; val w = size.width - pad * 2; val h = size.height - pad * 2
            repeat(5) { i -> drawLine(Color(0x33475569), Offset(pad, pad + h * i / 4), Offset(pad + w, pad + h * i / 4), strokeWidth = 1f) }
            fun path(values: List<Double?>): Path { val p = Path(); var s = false; values.forEachIndexed { i, v -> if (v == null) { s = false } else { val x = pad + w * i / max(1, values.size - 1); val y = pad + h - h * (v / 100.0).toFloat(); if (!s) { p.moveTo(x, y); s = true } else p.lineTo(x, y) } }; return p }
            drawPath(path(points.map { it.cpuUsage }), Orange, style = Stroke(3f))
            drawPath(path(points.map { it.memUsage }), Cyan, style = Stroke(3f))
        }
    }
}

@Composable fun NPTextField(value: String, onValue: (String) -> Unit, label: String, password: Boolean = false, onDone: () -> Unit = {}) {
    OutlinedTextField(value, onValue, label = { Text(label) }, singleLine = true, modifier = Modifier.fillMaxWidth(), keyboardActions = KeyboardActions(onDone = { onDone() }), visualTransformation = if (password) PasswordVisualTransformation() else VisualTransformation.None, colors = OutlinedTextFieldDefaults.colors(focusedTextColor = Text, unfocusedTextColor = Text, focusedBorderColor = Indigo, unfocusedBorderColor = Color(0xFF334155), focusedLabelColor = Muted, unfocusedLabelColor = Muted))
}

@Composable fun CardBlock(content: @Composable ColumnScope.() -> Unit) = Column(Modifier.fillMaxWidth().padding(vertical = 8.dp).clip(RoundedCornerShape(18.dp)).background(Card).padding(16.dp), content = content)
@Composable fun BackTitle(title: String, back: () -> Unit) = Row(Modifier.fillMaxWidth().padding(bottom = 14.dp), verticalAlignment = Alignment.CenterVertically) { NPButton(onClick = back) { Text("返回") }; Spacer(Modifier.width(16.dp)); Text(title, color = Text, fontSize = 28.sp, fontWeight = FontWeight.Bold) }
@Composable fun Legend(label: String, color: Color) = Row(verticalAlignment = Alignment.CenterVertically) { Box(Modifier.size(12.dp).clip(RoundedCornerShape(99.dp)).background(color)); Spacer(Modifier.width(6.dp)); Text(label, color = Muted) }

@Composable
fun NPButton(
    onClick: () -> Unit,
    enabled: Boolean = true,
    containerColor: Color = Indigo,
    contentColor: Color = Color.White,
    content: @Composable RowScope.() -> Unit
) = Button(
    onClick = onClick,
    enabled = enabled,
    colors = ButtonDefaults.buttonColors(
        containerColor = containerColor,
        contentColor = contentColor,
        disabledContainerColor = Color(0xFF334155),
        disabledContentColor = Muted
    ),
    content = content
)

@Composable
fun NPOutlinedButton(
    onClick: () -> Unit,
    enabled: Boolean = true,
    content: @Composable RowScope.() -> Unit
) = OutlinedButton(
    onClick = onClick,
    enabled = enabled,
    border = BorderStroke(1.dp, Muted.copy(alpha = 0.7f)),
    colors = ButtonDefaults.outlinedButtonColors(
        contentColor = Text,
        disabledContentColor = Muted
    ),
    content = content
)

@Composable
fun StatusDot(status: String, adminStatus: Int? = null) {
    if (adminStatus == 2) {
        Canvas(Modifier.size(18.dp)) {
            drawCircle(Red.copy(alpha = 0.18f), radius = size.minDimension / 2)
            drawLine(Red, Offset(size.width * 0.30f, size.height * 0.30f), Offset(size.width * 0.70f, size.height * 0.70f), strokeWidth = 3.5f, cap = StrokeCap.Round)
            drawLine(Red, Offset(size.width * 0.70f, size.height * 0.30f), Offset(size.width * 0.30f, size.height * 0.70f), strokeWidth = 3.5f, cap = StrokeCap.Round)
        }
        return
    }
    val c = when (status.lowercase()) { "online", "up" -> Green; "offline", "down" -> Red; else -> Orange }
    Box(Modifier.size(16.dp).clip(RoundedCornerShape(99.dp)).background(c))
}

@Composable fun StatsRow(devices: List<Device>) { val online = devices.count { it.status.lowercase() == "online" || it.status.lowercase() == "up" }; Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp)) { MiniStat("总数", devices.size.toString()); MiniStat("在线", online.toString()); MiniStat("离线", (devices.size - online).toString()) } }
@Composable fun RowScope.MiniStat(label: String, value: String) { Column(Modifier.weight(1f).clip(RoundedCornerShape(14.dp)).background(Card).padding(12.dp)) { Text(value, color = Text, fontSize = 24.sp, fontWeight = FontWeight.Bold); Text(label, color = Muted, fontSize = 12.sp) } }
@Composable fun DeviceRow(d: Device, open: (Long) -> Unit) = CardBlock { Row(Modifier.clickable { open(d.id) }.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) { StatusDot(d.status); Spacer(Modifier.width(12.dp)); Column(Modifier.weight(1f)) { Text(d.name.ifBlank { d.ip }, color = Text, fontSize = 20.sp, fontWeight = FontWeight.Bold); Text("${d.ip} · ${d.brand} · ${d.remark.ifBlank { "未备注" }}", color = Muted, maxLines = 1, overflow = TextOverflow.Ellipsis) } } }
@Composable fun PortRow(p: Port, open: (Long) -> Unit) = CardBlock { Row(Modifier.clickable { open(p.id) }.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) { StatusDot(if (p.operStatus == 1) "up" else "down", p.adminStatus); Spacer(Modifier.width(12.dp)); Column(Modifier.weight(1f)) { Text(displayPortName(p.name, p.customName, p.remark), color = Text, fontSize = 19.sp); Text("索引:${p.index} · ${p.speedMbps ?: 0} Mbps", color = Muted) } } }
@Composable fun SearchResults(items: List<SearchItem>, openDevice: (Long) -> Unit, openPort: (Long) -> Unit, clearFocus: () -> Unit) = CardBlock { Text("搜索结果", color = Text, fontWeight = FontWeight.Bold); items.take(20).forEach { item -> Row(Modifier.fillMaxWidth().clickable { clearFocus(); if (item.type == "port" && item.interfaceId != null) openPort(item.interfaceId) else item.deviceId?.let(openDevice) }.padding(vertical = 10.dp)) { Text(if (item.type == "port") "端口" else "资产", color = Indigo, modifier = Modifier.width(48.dp)); Column { Text(item.interfaceCustomName ?: item.interfaceName ?: item.deviceName ?: item.deviceIp ?: "-", color = Text); Text(item.snippet ?: item.deviceIp ?: "", color = Muted, maxLines = 1, overflow = TextOverflow.Ellipsis) } } } }
@Composable fun RangeButtons(selected: String, load: (String) -> Unit) = Row(
    Modifier.fillMaxWidth().padding(vertical = 10.dp).horizontalScroll(rememberScrollState()),
    horizontalArrangement = Arrangement.spacedBy(8.dp)
) {
    listOf("day" to "当日", "7d" to "近7天", "30d" to "近30天", "1y" to "近1年").forEach { (v, l) ->
        NPButton(
            onClick = { load(v) },
            containerColor = if (selected == v) Indigo else Color(0xFF334155),
            contentColor = Text
        ) { Text(l) }
    }
}

data class RangeSpec(val start: Instant, val end: Instant, val interval: String, val maxPoints: Int)

fun rangeSpec(range: String, custom: Pair<Instant, Instant>? = null): RangeSpec {
    val now = Instant.now()
    if (custom != null) {
        val spanSeconds = ChronoUnit.SECONDS.between(custom.first, custom.second).coerceAtLeast(1)
        val interval = when {
            spanSeconds <= 24 * 3600 -> "2m"
            spanSeconds <= 7 * 24 * 3600 -> "5m"
            spanSeconds <= 30L * 24 * 3600 -> "5m"
            spanSeconds <= 180L * 24 * 3600 -> "1h"
            else -> "1h"
        }
        return RangeSpec(custom.first, custom.second, interval, if (interval == "1h") 1500 else 4000)
    }
    val zdt = now.atZone(ServerZone)
    val start = when (range) {
        "7d" -> zdt.minusDays(6).toLocalDate().atStartOfDay(ServerZone).toInstant()
        "30d" -> zdt.minusDays(29).toLocalDate().atStartOfDay(ServerZone).toInstant()
        "1y" -> zdt.minusYears(1).truncatedTo(ChronoUnit.HOURS).toInstant()
        else -> zdt.toLocalDate().atStartOfDay(ServerZone).toInstant()
    }
    val interval = when (range) {
        "day" -> "2m"
        "7d" -> "5m"
        "30d" -> "5m"
        "1y" -> "1h"
        else -> "2m"
    }
    return RangeSpec(start, now, interval, if (range == "30d") 4000 else if (range == "1y") 1500 else 2500)
}

fun defaultCustomStartText(): String = formatLocalDateTime(Instant.now().minus(1, ChronoUnit.DAYS))
fun defaultCustomEndText(): String = formatLocalDateTime(Instant.now())
fun formatLocalDateTime(instant: Instant): String = DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm").format(instant.atZone(ServerZone))
fun parseCustomRange(start: String, end: String): Pair<Instant, Instant> {
    val f = DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm")
    val s = LocalDateTime.parse(start.trim(), f).atZone(ServerZone).toInstant()
    val e = LocalDateTime.parse(end.trim(), f).atZone(ServerZone).toInstant()
    require(e.isAfter(s)) { "结束时间必须晚于开始时间" }
    return s to e
}
fun rangeLabel(range: String): String = when (range) { "day" -> "当日"; "7d" -> "近7天"; "30d" -> "近30天"; "1y" -> "近1年"; "custom" -> "自定义"; else -> "" }

fun niceMax(v: Double): Double { val n = if (v <= 0.0) 100.0 else v * 1.18; val units = listOf(1.0, 2.0, 5.0, 10.0); var base = 1.0; while (base * 10 < n) base *= 10; return units.firstOrNull { it * base >= n }?.times(base) ?: 10.0 * base }
fun tierRank(d: Device): Int = when { d.remark.contains("核心") || d.name.contains("核心") -> 0; d.remark.contains("汇聚") || d.name.contains("汇聚") -> 1; else -> 2 }
fun Device.statusLabel() = when (status.lowercase()) { "online", "up" -> "在线"; "offline", "down" -> "离线"; else -> "未知" }

fun hasTrafficValue(point: TrafficHistoryPoint): Boolean =
    listOf(point.trafficInBps, point.trafficOutBps).any { it != null && !it.isNaN() }

fun formatReadoutBps(v: Double?): String = if (v == null || v.isNaN()) "-" else formatBps(v)

fun nearestTrafficIndex(points: List<TrafficHistoryPoint>, x: Float, canvasWidth: Float, padL: Float, padR: Float): Int? {
    if (points.isEmpty()) return null
    val w = (canvasWidth - padL - padR).coerceAtLeast(1f)
    val raw = (((x - padL) / w).coerceIn(0f, 1f) * (points.size - 1)).roundToInt()
    var bestIndex: Int? = null
    var bestDistance = Int.MAX_VALUE
    points.forEachIndexed { i, point ->
        if (hasTrafficValue(point)) {
            val distance = abs(i - raw)
            if (distance < bestDistance) {
                bestDistance = distance
                bestIndex = i
            }
        }
    }
    return bestIndex
}

fun saveTrafficChart(context: Context, points: List<TrafficHistoryPoint>, title: String, range: String, selectedIndex: Int? = null): Boolean {
    if (points.isEmpty()) return false
    val bitmap = renderTrafficBitmap(points, title, range, selectedIndex)
    return runCatching {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            val values = ContentValues().apply {
                put(MediaStore.Images.Media.DISPLAY_NAME, "NetPulse_${System.currentTimeMillis()}.png")
                put(MediaStore.Images.Media.MIME_TYPE, "image/png")
                put(MediaStore.Images.Media.RELATIVE_PATH, "Pictures/NetPulse")
            }
            val uri = context.contentResolver.insert(MediaStore.Images.Media.EXTERNAL_CONTENT_URI, values) ?: return false
            context.contentResolver.openOutputStream(uri)?.use { bitmap.compress(Bitmap.CompressFormat.PNG, 100, it) } ?: return false
        } else {
            @Suppress("DEPRECATION")
            MediaStore.Images.Media.insertImage(context.contentResolver, bitmap, "NetPulse_${System.currentTimeMillis()}", title)
        }
        true
    }.getOrDefault(false)
}

fun renderTrafficBitmap(points: List<TrafficHistoryPoint>, title: String, range: String, selectedIndex: Int? = null): Bitmap {
    val width = 1600
    val height = 900
    val bmp = Bitmap.createBitmap(width, height, Bitmap.Config.ARGB_8888)
    val canvas = AndroidCanvas(bmp)
    canvas.drawColor(AndroidColor.rgb(15, 23, 42))
    val textPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply { color = AndroidColor.WHITE; textSize = 42f; typeface = android.graphics.Typeface.DEFAULT_BOLD }
    val mutedPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply { color = AndroidColor.rgb(183, 195, 214); textSize = 26f }
    canvas.drawText(title, 70f, 70f, textPaint)
    val selected = selectedIndex?.takeIf { it in points.indices && hasTrafficValue(points[it]) }
    val selectedPoint = selected?.let { points[it] }
    canvas.drawText("$range · 入方向/出方向", 70f, 112f, mutedPaint)
    selectedPoint?.let {
        canvas.drawText("读数 ${formatTime(it.timestamp)}  入:${formatReadoutBps(it.trafficInBps)}  出:${formatReadoutBps(it.trafficOutBps)}", 70f, 148f, mutedPaint)
    }

    val left = 110f; val top = 190f; val right = width - 70f; val bottom = height - 110f
    val w = right - left; val h = bottom - top
    val values = points.flatMap { listOfNotNull(it.trafficInBps, it.trafficOutBps) }.filter { !it.isNaN() }
    val maxY = niceMax(values.maxOrNull() ?: 1.0)
    val gridPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply { color = AndroidColor.argb(80, 71, 85, 105); strokeWidth = 1.5f }
    val axisPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply { color = AndroidColor.rgb(148, 163, 184); textSize = 22f }
    repeat(5) { i ->
        val y = top + h * i / 4f
        canvas.drawLine(left, y, right, y, gridPaint)
        canvas.drawText(formatBps(maxY * (4 - i) / 4.0), 20f, y + 8f, axisPaint)
    }
    fun drawSeries(selector: (TrafficHistoryPoint) -> Double?, color: Int) {
        val p = AndroidPath(); var started = false
        points.forEachIndexed { i, point ->
            val v = selector(point)
            if (v == null || v.isNaN()) return@forEachIndexed
            val x = left + if (points.size <= 1) 0f else w * i / (points.size - 1).toFloat()
            val y = top + h - (h * (v / maxY).toFloat()).coerceIn(0f, h)
            if (!started) { p.moveTo(x, y); started = true } else p.lineTo(x, y)
        }
        val paint = Paint(Paint.ANTI_ALIAS_FLAG).apply { this.color = color; style = Paint.Style.STROKE; strokeWidth = 5f; strokeCap = Paint.Cap.ROUND; strokeJoin = Paint.Join.ROUND }
        canvas.drawPath(p, paint)
    }
    drawSeries({ it.trafficInBps }, Indigo.toArgb())
    drawSeries({ it.trafficOutBps }, Green.toArgb())
    selected?.let { idx ->
        val x = left + if (points.size <= 1) 0f else w * idx / (points.size - 1).toFloat()
        val markerPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply { color = AndroidColor.argb(170, 226, 232, 240); strokeWidth = 2.5f }
        canvas.drawLine(x, top, x, bottom, markerPaint)
        fun drawMarker(v: Double?, color: Int) {
            if (v == null || v.isNaN()) return
            val y = top + h - (h * (v / maxY).toFloat()).coerceIn(0f, h)
            val fill = Paint(Paint.ANTI_ALIAS_FLAG).apply { this.color = color; style = Paint.Style.FILL }
            val ring = Paint(Paint.ANTI_ALIAS_FLAG).apply { this.color = AndroidColor.rgb(15, 23, 42); style = Paint.Style.FILL }
            canvas.drawCircle(x, y, 10f, fill)
            canvas.drawCircle(x, y, 5f, ring)
        }
        drawMarker(points[idx].trafficInBps, Indigo.toArgb())
        drawMarker(points[idx].trafficOutBps, Green.toArgb())
    }
    val legendPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply { textSize = 28f }
    legendPaint.color = Indigo.toArgb(); canvas.drawCircle(right - 260f, 95f, 10f, legendPaint); canvas.drawText("入方向", right - 238f, 105f, mutedPaint)
    legendPaint.color = Green.toArgb(); canvas.drawCircle(right - 140f, 95f, 10f, legendPaint); canvas.drawText("出方向", right - 118f, 105f, mutedPaint)
    points.firstOrNull()?.let { canvas.drawText(formatTime(it.timestamp), left, height - 50f, axisPaint) }
    points.lastOrNull()?.let { canvas.drawText(formatTime(it.timestamp), right - 230f, height - 50f, axisPaint) }
    return bmp
}
