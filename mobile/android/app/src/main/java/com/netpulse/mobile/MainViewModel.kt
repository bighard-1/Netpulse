package com.netpulse.mobile

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import kotlinx.serialization.decodeFromString
import kotlinx.serialization.encodeToString
import kotlinx.coroutines.async
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.io.IOException
import java.net.SocketTimeoutException
import java.time.OffsetDateTime
import java.time.format.DateTimeFormatter
import java.util.concurrent.ConcurrentHashMap
import java.util.Collections
import java.util.LinkedHashMap

class MainViewModel(app: Application) : AndroidViewModel(app) {
    private val sp = app.getSharedPreferences("netpulse", 0)
    private val secureStore = SecureStore(app)

    private var base = sp.getString("base_url", "http://119.40.55.18:18080/api") ?: "http://119.40.55.18:18080/api"
    private var client = NetPulseClient(base) { secureStore.getToken() }

    private val _token = MutableStateFlow(secureStore.getToken())
    val token: StateFlow<String> = _token

    private val _devices = MutableStateFlow<List<DeviceStatus>>(emptyList())
    val devices: StateFlow<List<DeviceStatus>> = _devices

    private val _deviceDetail = MutableStateFlow<DeviceStatus?>(null)
    val deviceDetail: StateFlow<DeviceStatus?> = _deviceDetail

    private val _message = MutableStateFlow("")
    val message: StateFlow<String> = _message

    private val _loading = MutableStateFlow(false)
    val loading: StateFlow<Boolean> = _loading
    private val _detailLoading = MutableStateFlow(false)
    val detailLoading: StateFlow<Boolean> = _detailLoading
    private val _historyLoading = MutableStateFlow(false)
    val historyLoading: StateFlow<Boolean> = _historyLoading
    private val _portLoading = MutableStateFlow(false)
    val portLoading: StateFlow<Boolean> = _portLoading
    private val _queryProgress = MutableStateFlow("空闲")
    val queryProgress: StateFlow<String> = _queryProgress
    private val _lastSnapshotAt = MutableStateFlow("")
    val lastSnapshotAt: StateFlow<String> = _lastSnapshotAt
    private val _perfSummary = MutableStateFlow("P95: -")
    val perfSummary: StateFlow<String> = _perfSummary

    private val _cpu = MutableStateFlow<List<DeviceHistoryPoint>>(emptyList())
    val cpu: StateFlow<List<DeviceHistoryPoint>> = _cpu

    private val _mem = MutableStateFlow<List<DeviceHistoryPoint>>(emptyList())
    val mem: StateFlow<List<DeviceHistoryPoint>> = _mem

    private val _traffic = MutableStateFlow<List<InterfaceHistoryPoint>>(emptyList())
    val traffic: StateFlow<List<InterfaceHistoryPoint>> = _traffic

    private val _logs = MutableStateFlow<List<DeviceLog>>(emptyList())
    val logs: StateFlow<List<DeviceLog>> = _logs
    private val _auditLogs = MutableStateFlow<List<AuditLog>>(emptyList())
    val auditLogs: StateFlow<List<AuditLog>> = _auditLogs

    private val _quickPeekDevice = MutableStateFlow<DeviceStatus?>(null)
    val quickPeekDevice: StateFlow<DeviceStatus?> = _quickPeekDevice
    private var detailJob: Job? = null
    private var trafficJob: Job? = null
    private var degradedUntilMs: Long = 0
    private val apiCostMs = Collections.synchronizedList(mutableListOf<Long>())
    private val deviceDetailCache = Collections.synchronizedMap(
        object : LinkedHashMap<Long, Pair<Long, DeviceStatus>>(64, 0.75f, true) {
            override fun removeEldestEntry(eldest: MutableMap.MutableEntry<Long, Pair<Long, DeviceStatus>>?): Boolean = size > 80
        }
    )
    private val portTrafficCache = Collections.synchronizedMap(
        object : LinkedHashMap<String, Pair<Long, List<InterfaceHistoryPoint>>>(96, 0.75f, true) {
            override fun removeEldestEntry(eldest: MutableMap.MutableEntry<String, Pair<Long, List<InterfaceHistoryPoint>>>?): Boolean = size > 120
        }
    )
    private val cacheTTLms = 45_000L
    private val detailScrollAnchor = ConcurrentHashMap<Long, Pair<Int, Int>>()

    fun saveBaseUrl(url: String) {
        base = url.trim().ifBlank { "http://119.40.55.18:18080/api" }
        sp.edit().putString("base_url", base).apply()
        client = NetPulseClient(base) { secureStore.getToken() }
    }

    fun login(username: String, password: String) {
        viewModelScope.launch {
            _loading.value = true
            try {
                val t0 = System.currentTimeMillis()
                val res = withContext(Dispatchers.IO) { client.loginMobile(username, password) }
                trackApiCost(System.currentTimeMillis() - t0)
                secureStore.setToken(res.token)
                _token.value = res.token
                _message.value = "登录成功"
                refreshDevices()
            } catch (e: Exception) {
                _message.value = classifyError(e, "登录失败")
            } finally {
                _loading.value = false
            }
        }
    }

    fun biometricUnlock() {
        val t = secureStore.getToken()
        if (t.isBlank()) {
            _message.value = "请先使用用户名密码完成首次登录"
            return
        }
        _token.value = t
        refreshDevices()
    }

    fun logout() {
        secureStore.clearToken()
        _token.value = ""
        _devices.value = emptyList()
        _deviceDetail.value = null
        _cpu.value = emptyList()
        _mem.value = emptyList()
        _traffic.value = emptyList()
        _logs.value = emptyList()
    }

    private fun handleApiError(ex: Exception, fallback: String) {
        if (ex is ApiException && ex.code == 401) {
            _message.value = "登录已失效，请重新登录"
            logout()
        } else {
            _message.value = ex.message ?: fallback
        }
    }

    fun refreshDevices() {
        if (_token.value.isBlank()) return
        viewModelScope.launch {
            _loading.value = true
            try {
                val now = System.currentTimeMillis()
                val costStart = now
                _devices.value = withContext(Dispatchers.IO) { client.fetchDevices() }
                trackApiCost(System.currentTimeMillis() - costStart)
                val eventsStart = System.currentTimeMillis()
                _auditLogs.value = withContext(Dispatchers.IO) { client.fetchRecentEvents() }.take(5)
                trackApiCost(System.currentTimeMillis() - eventsStart)
                _lastSnapshotAt.value = OffsetDateTime.now().toString()
                persistSnapshot(_devices.value)
            } catch (e: Exception) {
                val snapshot = restoreSnapshot()
                if (snapshot.isNotEmpty()) {
                    _devices.value = snapshot
                    _message.value = "弱网/离线：已展示最近快照（${_lastSnapshotAt.value.ifBlank { "未知时间" }}）"
                } else {
                    handleApiError(e, "加载设备失败")
                }
            } finally {
                _loading.value = false
            }
        }
    }

    fun loadDeviceDetail(deviceId: Long, start: OffsetDateTime, end: OffsetDateTime) {
        if (_token.value.isBlank()) return
        detailJob?.cancel()
        detailJob = viewModelScope.launch {
            _detailLoading.value = true
            _historyLoading.value = false
            try {
                val now = System.currentTimeMillis()
                val cached = deviceDetailCache[deviceId]
                if (cached != null && now-cached.first <= cacheTTLms) {
                    _deviceDetail.value = cached.second
                }
                _queryProgress.value = "加载中 1/3：设备基础信息"
                val t1 = System.currentTimeMillis()
                val detail = withContext(Dispatchers.IO) { client.fetchDeviceById(deviceId) }
                trackApiCost(System.currentTimeMillis() - t1)
                _deviceDetail.value = detail
                deviceDetailCache[deviceId] = now to detail
                _detailLoading.value = false

                _historyLoading.value = true
                _queryProgress.value = "加载中 2/3：性能曲线"
                val s = DateTimeFormatter.ISO_OFFSET_DATE_TIME.format(start)
                val e = DateTimeFormatter.ISO_OFFSET_DATE_TIME.format(end)
                val cpuDeferred = async(Dispatchers.IO) { client.fetchDeviceHistory("cpu", deviceId, s, e) }
                val memDeferred = async(Dispatchers.IO) { client.fetchDeviceHistory("mem", deviceId, s, e) }
                _cpu.value = cpuDeferred.await()
                _mem.value = memDeferred.await()
                _queryProgress.value = "加载中 3/3：设备日志"
                val logsDeferred = async(Dispatchers.IO) { runCatching { client.fetchLogs(deviceId) }.getOrDefault(emptyList()) }
                _logs.value = logsDeferred.await()
                _message.value = ""
                _queryProgress.value = "完成"
            } catch (ex: Exception) {
                if (ex is CancellationException) return@launch
                handleApiError(ex, "加载详情失败")
                _queryProgress.value = "失败"
            } finally {
                _detailLoading.value = false
                _historyLoading.value = false
            }
        }
    }

    fun loadPortTraffic(portId: Long, start: OffsetDateTime, end: OffsetDateTime, forceDetailed: Boolean = false) {
        if (_token.value.isBlank()) return
        trafficJob?.cancel()
        trafficJob = viewModelScope.launch {
            _portLoading.value = true
            try {
                val s = DateTimeFormatter.ISO_OFFSET_DATE_TIME.format(start)
                val e = DateTimeFormatter.ISO_OFFSET_DATE_TIME.format(end)
                val tier = trafficTier(start, end, forceDetailed)
                val key = "$portId|$s|$e|${tier.interval}|${tier.maxPoints}"
                val now = System.currentTimeMillis()
                val cached = portTrafficCache[key]
                if (cached != null && now-cached.first <= cacheTTLms) {
                    _queryProgress.value = "命中缓存 1/1"
                    _traffic.value = cached.second
                } else {
                    _queryProgress.value = "加载中 1/2：请求流量数据"
                    val t0 = System.currentTimeMillis()
                    val data = withContext(Dispatchers.IO) { client.fetchTrafficHistory(portId, s, e, tier.interval, tier.maxPoints) }
                    trackApiCost(System.currentTimeMillis() - t0)
                    _queryProgress.value = "加载中 2/2：图表预处理"
                    _traffic.value = data
                    portTrafficCache[key] = now to data
                }
                _queryProgress.value = "完成"
            } catch (ex: Exception) {
                if (ex is CancellationException) return@launch
                handleApiError(ex, "加载端口流量失败")
                _queryProgress.value = "失败"
            } finally {
                _portLoading.value = false
            }
        }
    }

    fun cancelPortTrafficLoading() {
        trafficJob?.cancel()
        _portLoading.value = false
        _queryProgress.value = "已取消"
    }

    private fun historyInterval(start: OffsetDateTime, end: OffsetDateTime): String {
        val days = kotlin.math.max(1L, java.time.Duration.between(start, end).toDays())
        return when {
            days > 180 -> "1h"
            days > 30 -> "5m"
            else -> "1m"
        }
    }

    private fun historyMaxPoints(start: OffsetDateTime, end: OffsetDateTime): Int {
        val days = kotlin.math.max(1L, java.time.Duration.between(start, end).toDays())
        return when {
            days > 365 * 2 -> 900
            days > 180 -> 1200
            days > 30 -> 1800
            else -> 2600
        }
    }

    private data class TrafficTier(val interval: String, val maxPoints: Int)
    private fun trafficTier(start: OffsetDateTime, end: OffsetDateTime, forceDetailed: Boolean): TrafficTier {
        val days = kotlin.math.max(1L, java.time.Duration.between(start, end).toDays())
        return when {
            forceDetailed -> TrafficTier("5m", 2600)
            days > 365 * 2 -> TrafficTier("6h", 480)
            days > 30 -> TrafficTier("1h", 1200)
            days > 7 -> TrafficTier("5m", 1600)
            else -> TrafficTier("1m", 2400)
        }
    }

    fun updateInterfaceRemark(deviceId: Long, interfaceId: Long, remark: String, start: OffsetDateTime, end: OffsetDateTime) {
        if (_token.value.isBlank()) return
        viewModelScope.launch {
            try {
                withContext(Dispatchers.IO) { client.updateInterfaceRemark(interfaceId, remark) }
                _message.value = "端口备注已更新"
                loadDeviceDetail(deviceId, start, end)
            } catch (ex: Exception) {
                handleApiError(ex, "更新端口备注失败")
            }
        }
    }

    fun updateDeviceRemark(deviceId: Long, remark: String) {
        if (_token.value.isBlank()) return
        viewModelScope.launch {
            try {
                withContext(Dispatchers.IO) { client.updateDeviceRemark(deviceId, remark) }
                _message.value = "设备备注已更新"
                refreshDevices()
            } catch (ex: Exception) {
                handleApiError(ex, "更新设备备注失败")
            }
        }
    }

    fun updateDeviceProfile(deviceId: Long, name: String, brand: String, remark: String, maintenanceMode: Boolean) {
        if (_token.value.isBlank()) return
        viewModelScope.launch {
            try {
                withContext(Dispatchers.IO) { client.updateDeviceProfile(deviceId, name, brand, remark, maintenanceMode) }
                _message.value = "资产信息已更新"
                refreshDevices()
            } catch (ex: Exception) {
                handleApiError(ex, "更新资产失败")
            }
        }
    }


    fun updateMaintenanceMode(deviceId: Long, enabled: Boolean, start: OffsetDateTime, end: OffsetDateTime) {
        if (_token.value.isBlank()) return
        viewModelScope.launch {
            try {
                val d = _deviceDetail.value ?: withContext(Dispatchers.IO) { client.fetchDeviceById(deviceId) }
                withContext(Dispatchers.IO) { client.updateDevice(d, enabled) }
                _message.value = if (enabled) "已开启维护模式" else "已关闭维护模式"
                loadDeviceDetail(deviceId, start, end)
                refreshDevices()
            } catch (ex: Exception) {
                handleApiError(ex, "更新维护模式失败")
            }
        }
    }


    fun openQuickPeek(deviceId: Long) {
        viewModelScope.launch {
            val d = _devices.value.firstOrNull { it.id == deviceId }
            _quickPeekDevice.value = d
            if (d != null) loadDeviceDetail(d.id, OffsetDateTime.now().minusDays(1), OffsetDateTime.now())
        }
    }

    fun closeQuickPeek() {
        _quickPeekDevice.value = null
    }

    fun saveDetailScroll(deviceId: Long, index: Int, offset: Int) {
        detailScrollAnchor[deviceId] = index to offset
    }

    fun getDetailScroll(deviceId: Long): Pair<Int, Int> {
        return detailScrollAnchor[deviceId] ?: (0 to 0)
    }

    private fun classifyError(ex: Exception, fallback: String): String {
        if (ex is ApiException) {
            return when (ex.code) {
                401 -> "登录过期，请重新登录"
                403 -> "权限不足，无法访问该数据"
                404 -> "目标不存在或已被删除"
                408 -> "请求超时，请重试"
                else -> ex.message.ifBlank { "$fallback（${ex.code}）" }
            }
        }
        return when (ex) {
            is SocketTimeoutException -> "网络超时，请稍后重试"
            is IOException -> "网络连接异常，请检查网络"
            else -> ex.message ?: fallback
        }
    }

    private fun trackApiCost(costMs: Long) {
        if (costMs <= 0) return
        synchronized(apiCostMs) {
            apiCostMs += costMs
            while (apiCostMs.size > 120) apiCostMs.removeAt(0)
            val sorted = apiCostMs.sorted()
            val p95Index = ((sorted.size - 1) * 0.95).toInt().coerceIn(0, sorted.lastIndex)
            _perfSummary.value = "P95: ${sorted[p95Index]}ms / 最近${sorted.size}次"
        }
    }

    private fun persistSnapshot(devices: List<DeviceStatus>) {
        runCatching {
            val json = kotlinx.serialization.json.Json { ignoreUnknownKeys = true }
            sp.edit()
                .putString("snapshot_devices", json.encodeToString(devices))
                .putString("snapshot_time", OffsetDateTime.now().toString())
                .apply()
            _lastSnapshotAt.value = sp.getString("snapshot_time", "") ?: ""
        }
    }

    private fun restoreSnapshot(): List<DeviceStatus> {
        val raw = sp.getString("snapshot_devices", "") ?: ""
        _lastSnapshotAt.value = sp.getString("snapshot_time", "") ?: ""
        if (raw.isBlank()) return emptyList()
        return runCatching { kotlinx.serialization.json.Json { ignoreUnknownKeys = true }.decodeFromString<List<DeviceStatus>>(raw) }
            .getOrDefault(emptyList())
    }
}
