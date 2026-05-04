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
    private var base = sp.getString("base_url", "http://119.40.55.18:18080/api") ?: "http://119.40.55.18:18080/api"
    private var client = NetPulseClient(base)

    private val _token = MutableStateFlow(sp.getString("token", "") ?: "")
    val token: StateFlow<String> = _token

    private val _devices = MutableStateFlow<List<DeviceStatus>>(emptyList())
    val devices: StateFlow<List<DeviceStatus>> = _devices

    private val _logs = MutableStateFlow<List<DeviceLog>>(emptyList())
    val logs: StateFlow<List<DeviceLog>> = _logs

    private val _message = MutableStateFlow("")
    val message: StateFlow<String> = _message

    private val _loading = MutableStateFlow(false)
    val loading: StateFlow<Boolean> = _loading

    private val _cpu = MutableStateFlow<List<DeviceHistoryPoint>>(emptyList())
    val cpu: StateFlow<List<DeviceHistoryPoint>> = _cpu
    private val _mem = MutableStateFlow<List<DeviceHistoryPoint>>(emptyList())
    val mem: StateFlow<List<DeviceHistoryPoint>> = _mem

    fun saveBaseUrl(url: String) {
        base = url.trim().ifBlank { "http://119.40.55.18:18080/api" }
        sp.edit().putString("base_url", base).apply()
        client = NetPulseClient(base)
    }
    fun loadSavedCreds(): Pair<String, String> = (sp.getString("u", "") ?: "") to (sp.getString("p", "") ?: "")

    fun login(username: String, password: String, rememberCreds: Boolean = true) {
        viewModelScope.launch {
            _loading.value = true
            try {
                val res = withContext(Dispatchers.IO) { client.loginMobile(username, password) }
                _token.value = res.token
                sp.edit().putString("token", res.token).apply()
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
        _token.value = ""
        _devices.value = emptyList()
        _logs.value = emptyList()
        sp.edit().remove("token").apply()
    }

    fun refreshDevices() {
        val t = _token.value
        if (t.isBlank()) return
        viewModelScope.launch {
            _loading.value = true
            try {
                _devices.value = withContext(Dispatchers.IO) { client.fetchDevices(t) }
            } catch (e: Exception) {
                _message.value = e.message ?: "加载设备失败"
            } finally {
                _loading.value = false
            }
        }
    }

    fun loadDeviceDetail(device: DeviceStatus, start: OffsetDateTime, end: OffsetDateTime) {
        val t = _token.value
        if (t.isBlank()) return
        viewModelScope.launch {
            _loading.value = true
            try {
                val s = DateTimeFormatter.ISO_OFFSET_DATE_TIME.format(start)
                val e = DateTimeFormatter.ISO_OFFSET_DATE_TIME.format(end)
                _cpu.value = withContext(Dispatchers.IO) { client.fetchDeviceHistory(t, "cpu", device.id, s, e) }
                _mem.value = withContext(Dispatchers.IO) { client.fetchDeviceHistory(t, "mem", device.id, s, e) }
                _logs.value = withContext(Dispatchers.IO) { client.fetchLogs(t, device.id) }
            } catch (ex: Exception) {
                _message.value = ex.message ?: "加载详情失败"
            } finally {
                _loading.value = false
            }
        }
    }

    fun updateInterfaceRemark(interfaceId: Long, remark: String, done: () -> Unit = {}) {
        val t = _token.value
        if (t.isBlank()) return
        viewModelScope.launch {
            try {
                withContext(Dispatchers.IO) { client.updateInterfaceRemark(t, interfaceId, remark) }
                _message.value = "端口备注已更新"
                done()
            } catch (e: Exception) {
                _message.value = e.message ?: "更新失败"
            }
        }
    }
}
