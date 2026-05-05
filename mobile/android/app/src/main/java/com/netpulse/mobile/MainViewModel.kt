package com.netpulse.mobile

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.time.OffsetDateTime
import java.time.format.DateTimeFormatter

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

    fun saveBaseUrl(url: String) {
        base = url.trim().ifBlank { "http://119.40.55.18:18080/api" }
        sp.edit().putString("base_url", base).apply()
        client = NetPulseClient(base) { secureStore.getToken() }
    }

    fun loadSavedCreds(): Pair<String, String> = (sp.getString("u", "") ?: "") to (sp.getString("p", "") ?: "")

    fun login(username: String, password: String, rememberCreds: Boolean = true) {
        viewModelScope.launch {
            _loading.value = true
            try {
                val res = withContext(Dispatchers.IO) { client.loginMobile(username, password) }
                secureStore.setToken(res.token)
                _token.value = res.token
                if (rememberCreds) sp.edit().putString("u", username).putString("p", password).apply()
                _message.value = "登录成功"
                refreshDevices()
            } catch (e: Exception) {
                _message.value = e.message ?: "登录失败"
            } finally {
                _loading.value = false
            }
        }
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
                _devices.value = withContext(Dispatchers.IO) { client.fetchDevices() }
                _auditLogs.value = withContext(Dispatchers.IO) { client.fetchAuditLogs() }.take(5)
            } catch (e: Exception) {
                handleApiError(e, "加载设备失败")
            } finally {
                _loading.value = false
            }
        }
    }

    fun loadDeviceDetail(deviceId: Long, start: OffsetDateTime, end: OffsetDateTime) {
        if (_token.value.isBlank()) return
        viewModelScope.launch {
            _loading.value = true
            try {
                val s = DateTimeFormatter.ISO_OFFSET_DATE_TIME.format(start)
                val e = DateTimeFormatter.ISO_OFFSET_DATE_TIME.format(end)
                _deviceDetail.value = withContext(Dispatchers.IO) { client.fetchDeviceById(deviceId) }
                _cpu.value = withContext(Dispatchers.IO) { client.fetchDeviceHistory("cpu", deviceId, s, e) }
                _mem.value = withContext(Dispatchers.IO) { client.fetchDeviceHistory("mem", deviceId, s, e) }
                _logs.value = withContext(Dispatchers.IO) { client.fetchLogs(deviceId) }
            } catch (ex: Exception) {
                handleApiError(ex, "加载详情失败")
            } finally {
                _loading.value = false
            }
        }
    }

    fun loadPortTraffic(portId: Long, start: OffsetDateTime, end: OffsetDateTime) {
        if (_token.value.isBlank()) return
        viewModelScope.launch {
            _loading.value = true
            try {
                val s = DateTimeFormatter.ISO_OFFSET_DATE_TIME.format(start)
                val e = DateTimeFormatter.ISO_OFFSET_DATE_TIME.format(end)
                _traffic.value = withContext(Dispatchers.IO) { client.fetchTrafficHistory(portId, s, e) }
            } catch (ex: Exception) {
                handleApiError(ex, "加载端口流量失败")
            } finally {
                _loading.value = false
            }
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
}
