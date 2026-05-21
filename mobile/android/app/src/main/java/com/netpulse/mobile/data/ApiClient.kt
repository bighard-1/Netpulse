package com.netpulse.mobile.data

import kotlinx.serialization.builtins.ListSerializer
import kotlinx.serialization.builtins.serializer
import kotlinx.serialization.json.Json
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import java.net.URLEncoder
import java.time.Instant
import java.util.concurrent.TimeUnit

class ApiClient(private val session: SessionStore) {
    private val json = Json { ignoreUnknownKeys = true }
    private val http = OkHttpClient.Builder()
        .connectTimeout(15, TimeUnit.SECONDS)
        .readTimeout(25, TimeUnit.SECONDS)
        .build()
    private val media = "application/json; charset=utf-8".toMediaType()

    private fun url(path: String, query: Map<String, String?> = emptyMap()): String {
        val base = session.baseUrl.trimEnd('/')
        val qs = query.filterValues { !it.isNullOrBlank() }.map { (k, v) ->
            "${enc(k)}=${enc(v.orEmpty())}"
        }.joinToString("&")
        return base + path + if (qs.isBlank()) "" else "?$qs"
    }

    private fun enc(v: String) = URLEncoder.encode(v, Charsets.UTF_8.name())

    private fun request(path: String, query: Map<String, String?> = emptyMap(), method: String = "GET", body: String? = null): Request {
        val builder = Request.Builder().url(url(path, query))
        if (session.token.isNotBlank()) builder.addHeader("Authorization", "Bearer ${session.token}")
        if (method != "GET") builder.method(method, body.orEmpty().toRequestBody(media))
        return builder.build()
    }

    private fun send(req: Request): String {
        http.newCall(req).execute().use { resp ->
            val text = resp.body?.string().orEmpty()
            if (resp.code == 401) {
                session.clear()
                throw IllegalStateException("登录已过期，请重新登录")
            }
            if (!resp.isSuccessful) throw IllegalStateException(text.ifBlank { "HTTP ${resp.code}" })
            return text
        }
    }

    fun login(username: String, password: String): LoginResponse {
        val body = json.encodeToString(LoginRequest.serializer(), LoginRequest(username, password))
        val first = runCatching { send(request("/auth/mobile/login", method = "POST", body = body)) }
            .recoverCatching { send(request("/login", method = "POST", body = body)) }
            .getOrThrow()
        val resp = json.decodeFromString(LoginResponse.serializer(), first)
        session.token = resp.token
        return resp
    }

    fun fetchDevices(): List<Device> = json.decodeFromString(ListSerializer(Device.serializer()), send(request("/devices")))

    fun fetchDevice(id: Long): Device = json.decodeFromString(Device.serializer(), send(request("/devices/$id")))

    fun fetchPort(id: Long): Port = json.decodeFromString(Port.serializer(), send(request("/interfaces/$id")))

    fun search(q: String): List<SearchItem> = if (q.isBlank()) emptyList() else json.decodeFromString(
        ListSerializer(SearchItem.serializer()),
        send(request("/search", mapOf("q" to q)))
    )

    fun fetchDeviceHistory(type: String, id: Long, start: Instant, end: Instant, maxPoints: Int, interval: String): List<DeviceHistoryPoint> {
        val text = send(request("/metrics/history", mapOf(
            "type" to type,
            "id" to id.toString(),
            "start" to start.toString(),
            "end" to end.toString(),
            "max_points" to maxPoints.toString(),
            "interval" to interval
        )))
        return json.decodeFromString(HistoryResponse.serializer(DeviceHistoryPoint.serializer()), text).data
    }

    fun fetchTraffic(id: Long, start: Instant, end: Instant, maxPoints: Int, interval: String): List<TrafficHistoryPoint> {
        val text = send(request("/metrics/history", mapOf(
            "type" to "traffic",
            "id" to id.toString(),
            "start" to start.toString(),
            "end" to end.toString(),
            "max_points" to maxPoints.toString(),
            "interval" to interval
        )))
        return json.decodeFromString(HistoryResponse.serializer(TrafficHistoryPoint.serializer()), text).data
    }
}
