package com.netpulse.mobile

import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import java.net.URLEncoder
import java.nio.charset.StandardCharsets

class NetPulseClient(
    private val baseUrl: String,
    private val http: OkHttpClient = OkHttpClient(),
    private val json: Json = Json { ignoreUnknownKeys = true }
) {
    private fun reqBuilder(path: String, token: String? = null): Request.Builder {
        val b = Request.Builder().url("$baseUrl$path")
        if (!token.isNullOrBlank()) b.header("Authorization", "Bearer $token")
        return b
    }

    fun loginMobile(username: String, password: String): LoginResponse {
        val body = json.encodeToString(LoginRequest(username, password))
            .toRequestBody("application/json".toMediaType())
        val req = reqBuilder("/auth/mobile/login").post(body).build()
        http.newCall(req).execute().use { resp ->
            require(resp.isSuccessful) { "登录失败(${resp.code})" }
            return json.decodeFromString(resp.body!!.string())
        }
    }

    fun fetchDevices(token: String): List<DeviceStatus> {
        val req = reqBuilder("/devices", token).get().build()
        http.newCall(req).execute().use { resp ->
            require(resp.isSuccessful) { "获取设备失败(${resp.code})" }
            return json.decodeFromString(resp.body!!.string())
        }
    }

    fun fetchLogs(token: String, deviceID: Long): List<DeviceLog> {
        val req = reqBuilder("/devices/$deviceID/logs", token).get().build()
        http.newCall(req).execute().use { resp ->
            require(resp.isSuccessful) { "获取日志失败(${resp.code})" }
            return json.decodeFromString(resp.body!!.string())
        }
    }

    fun updateInterfaceRemark(token: String, interfaceID: Long, remark: String) {
        val payload = json.encodeToString(mapOf("remark" to remark))
            .toRequestBody("application/json".toMediaType())
        val req = reqBuilder("/interfaces/$interfaceID/remark", token).put(payload).build()
        http.newCall(req).execute().use { resp ->
            require(resp.isSuccessful) { "更新端口备注失败(${resp.code})" }
        }
    }

    fun fetchDeviceHistory(token: String, type: String, deviceID: Long, start: String, end: String): List<DeviceHistoryPoint> {
        val s = URLEncoder.encode(start, StandardCharsets.UTF_8)
        val e = URLEncoder.encode(end, StandardCharsets.UTF_8)
        val req = reqBuilder("/metrics/history?type=$type&id=$deviceID&start=$s&end=$e", token).get().build()
        http.newCall(req).execute().use { resp ->
            require(resp.isSuccessful) { "获取历史失败(${resp.code})" }
            return json.decodeFromString<HistoryResponse<DeviceHistoryPoint>>(resp.body!!.string()).data
        }
    }

    fun fetchTrafficHistory(token: String, interfaceID: Long, start: String, end: String): List<InterfaceHistoryPoint> {
        val s = URLEncoder.encode(start, StandardCharsets.UTF_8)
        val e = URLEncoder.encode(end, StandardCharsets.UTF_8)
        val req = reqBuilder("/metrics/history?type=traffic&id=$interfaceID&start=$s&end=$e", token).get().build()
        http.newCall(req).execute().use { resp ->
            require(resp.isSuccessful) { "获取端口流量失败(${resp.code})" }
            return json.decodeFromString<HistoryResponse<InterfaceHistoryPoint>>(resp.body!!.string()).data
        }
    }
}
