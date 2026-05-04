package com.netpulse.mobile

import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json
import okhttp3.Interceptor
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import java.net.URLEncoder
import java.nio.charset.StandardCharsets

class ApiException(val code: Int, override val message: String) : RuntimeException(message)

class NetPulseClient(
    private val baseUrl: String,
    tokenProvider: () -> String
) {
    private val json: Json = Json { ignoreUnknownKeys = true }

    private val http: OkHttpClient = OkHttpClient.Builder()
        .addInterceptor(Interceptor { chain ->
            val req = chain.request()
            val token = tokenProvider()
            val withAuth = if (token.isNotBlank()) {
                req.newBuilder().header("Authorization", "Bearer $token").build()
            } else req
            chain.proceed(withAuth)
        })
        .build()

    private fun reqBuilder(path: String): Request.Builder = Request.Builder().url("$baseUrl$path")

    private fun ensureOk(respCode: Int, msg: String) {
        if (respCode in 200..299) return
        throw ApiException(respCode, "$msg($respCode)")
    }

    fun loginMobile(username: String, password: String): LoginResponse {
        val body = json.encodeToString(LoginRequest(username, password))
            .toRequestBody("application/json".toMediaType())
        val req = reqBuilder("/auth/mobile/login").post(body).build()
        http.newCall(req).execute().use { resp ->
            ensureOk(resp.code, "登录失败")
            return json.decodeFromString(resp.body!!.string())
        }
    }

    fun fetchDevices(): List<DeviceStatus> {
        val req = reqBuilder("/devices").get().build()
        http.newCall(req).execute().use { resp ->
            ensureOk(resp.code, "获取设备失败")
            return json.decodeFromString(resp.body!!.string())
        }
    }

    fun fetchDeviceById(deviceID: Long): DeviceStatus {
        val req = reqBuilder("/devices/$deviceID").get().build()
        http.newCall(req).execute().use { resp ->
            ensureOk(resp.code, "获取设备详情失败")
            return json.decodeFromString(resp.body!!.string())
        }
    }

    fun fetchLogs(deviceID: Long): List<DeviceLog> {
        val req = reqBuilder("/devices/$deviceID/logs").get().build()
        http.newCall(req).execute().use { resp ->
            ensureOk(resp.code, "获取日志失败")
            return json.decodeFromString(resp.body!!.string())
        }
    }

    fun fetchDeviceHistory(type: String, deviceID: Long, start: String, end: String): List<DeviceHistoryPoint> {
        val s = URLEncoder.encode(start, StandardCharsets.UTF_8)
        val e = URLEncoder.encode(end, StandardCharsets.UTF_8)
        val req = reqBuilder("/metrics/history?type=$type&id=$deviceID&start=$s&end=$e").get().build()
        http.newCall(req).execute().use { resp ->
            ensureOk(resp.code, "获取历史失败")
            return json.decodeFromString<HistoryResponse<DeviceHistoryPoint>>(resp.body!!.string()).data
        }
    }

    fun fetchTrafficHistory(interfaceID: Long, start: String, end: String): List<InterfaceHistoryPoint> {
        val s = URLEncoder.encode(start, StandardCharsets.UTF_8)
        val e = URLEncoder.encode(end, StandardCharsets.UTF_8)
        val req = reqBuilder("/metrics/history?type=traffic&id=$interfaceID&start=$s&end=$e").get().build()
        http.newCall(req).execute().use { resp ->
            ensureOk(resp.code, "获取端口流量失败")
            return json.decodeFromString<HistoryResponse<InterfaceHistoryPoint>>(resp.body!!.string()).data
        }
    }

    fun updateInterfaceRemark(interfaceID: Long, remark: String) {
        val body = """{"remark":${json.encodeToString(remark)}}"""
            .toRequestBody("application/json".toMediaType())
        val req = reqBuilder("/interfaces/$interfaceID/remark").put(body).build()
        http.newCall(req).execute().use { resp ->
            ensureOk(resp.code, "更新端口备注失败")
        }
    }
}
