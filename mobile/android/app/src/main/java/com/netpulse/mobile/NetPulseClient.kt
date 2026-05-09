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
import java.util.concurrent.TimeUnit

class ApiException(val code: Int, override val message: String) : RuntimeException(message)

class NetPulseClient(
    private val baseUrl: String,
    tokenProvider: () -> String
) {
    private val json: Json = Json { ignoreUnknownKeys = true }

    private val http: OkHttpClient = OkHttpClient.Builder()
        .connectTimeout(8, TimeUnit.SECONDS)
        .readTimeout(15, TimeUnit.SECONDS)
        .callTimeout(20, TimeUnit.SECONDS)
        .addInterceptor(Interceptor { chain ->
            val req = chain.request()
            val start = System.nanoTime()
            val token = tokenProvider()
            val withAuth = if (token.isNotBlank()) {
                req.newBuilder().header("Authorization", "Bearer $token").build()
            } else req
            val resp = chain.proceed(withAuth)
            val costMs = TimeUnit.NANOSECONDS.toMillis(System.nanoTime() - start)
            if (costMs > 1200) {
                android.util.Log.w("NetPulseClient", "slow-api ${withAuth.method} ${withAuth.url.encodedPath} ${costMs}ms")
            }
            resp
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

    fun fetchRecentEvents(): List<AuditLog> {
        val req = reqBuilder("/events/recent?limit=5").get().build()
        http.newCall(req).execute().use { resp ->
            ensureOk(resp.code, "获取事件流失败")
            val raw = resp.body!!.string()
            return try {
                json.decodeFromString<RecentEventsResponse>(raw).data
            } catch (_: Exception) {
                json.decodeFromString(raw)
            }
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

    fun fetchTrafficHistory(interfaceID: Long, start: String, end: String, interval: String? = null, maxPoints: Int? = null): List<InterfaceHistoryPoint> {
        val s = URLEncoder.encode(start, StandardCharsets.UTF_8)
        val e = URLEncoder.encode(end, StandardCharsets.UTF_8)
        val iv = interval?.takeIf { it.isNotBlank() }?.let { "&interval=${URLEncoder.encode(it, StandardCharsets.UTF_8)}" } ?: ""
        val mp = maxPoints?.takeIf { it > 0 }?.let { "&max_points=$it" } ?: ""
        val req = reqBuilder("/metrics/history?type=traffic&id=$interfaceID&start=$s&end=$e$iv$mp").get().build()
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



    fun updateDevice(device: DeviceStatus, maintenanceMode: Boolean) {
        val body = """{"name":${json.encodeToString(device.name)},"brand":${json.encodeToString(device.brand)},"remark":${json.encodeToString(device.remark)},"maintenance_mode":$maintenanceMode}"""
            .toRequestBody("application/json".toMediaType())
        val req = reqBuilder("/devices/${device.id}").put(body).build()
        http.newCall(req).execute().use { resp ->
            ensureOk(resp.code, "更新维护模式失败")
        }
    }
    fun updateDeviceRemark(deviceID: Long, remark: String) {
        val body = """{"remark":${json.encodeToString(remark)}}"""
            .toRequestBody("application/json".toMediaType())
        val req = reqBuilder("/devices/$deviceID/remark").put(body).build()
        http.newCall(req).execute().use { resp ->
            ensureOk(resp.code, "更新设备备注失败")
        }
    }

    fun updateDeviceProfile(deviceID: Long, name: String, brand: String, remark: String, maintenanceMode: Boolean) {
        val body = """{"name":${json.encodeToString(name)},"brand":${json.encodeToString(brand)},"remark":${json.encodeToString(remark)},"maintenance_mode":$maintenanceMode}"""
            .toRequestBody("application/json".toMediaType())
        val req = reqBuilder("/devices/$deviceID").put(body).build()
        http.newCall(req).execute().use { resp ->
            ensureOk(resp.code, "更新资产失败")
        }
    }
}

@kotlinx.serialization.Serializable
private data class RecentEventsResponse(val data: List<AuditLog> = emptyList())
