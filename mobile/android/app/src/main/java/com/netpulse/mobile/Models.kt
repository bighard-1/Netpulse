package com.netpulse.mobile

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

@Serializable
data class DeviceStatus(
    val id: Long,
    val ip: String,
    val name: String = "",
    val brand: String,
    val community: String? = null,
    val remark: String,
    @SerialName("created_at") val createdAt: String,
    @SerialName("last_metric_at") val lastMetricAt: String? = null,
    val status: String,
    @SerialName("maintenance_mode") val maintenanceMode: Boolean = false,
    @SerialName("status_reason") val statusReason: String? = null,
    val interfaces: List<NetInterface> = emptyList()
)

@Serializable
data class NetInterface(
    val id: Long,
    @SerialName("device_id") val deviceId: Long? = null,
    @SerialName("index") val index: Int,
    val name: String,
    val remark: String,
    @SerialName("custom_name") val customName: String? = null,
    @SerialName("oper_status") val operStatus: Int? = null
)

@Serializable
data class DeviceLog(
    val id: Long,
    @SerialName("device_id") val deviceId: Long,
    val level: String,
    val message: String,
    @SerialName("created_at") val createdAt: String
)

@Serializable
data class AuditLog(
    val id: Long,
    val action: String,
    val target: String? = null,
    val timestamp: String,
    @SerialName("status_code") val statusCode: Int? = null
)

@Serializable
data class InterfaceHistoryPoint(
    val timestamp: String,
    @SerialName("traffic_in_bps") val trafficInBps: Double? = null,
    @SerialName("traffic_out_bps") val trafficOutBps: Double? = null
)

@Serializable
data class DeviceHistoryPoint(
    val timestamp: String,
    @SerialName("cpu_usage") val cpuUsage: Double? = null,
    @SerialName("mem_usage") val memUsage: Double? = null
)

@Serializable
data class LoginRequest(val username: String, val password: String)

@Serializable
data class LoginResponse(val token: String, val user: LoginUser)

@Serializable
data class LoginUser(val username: String, val role: String)

@Serializable
data class HistoryResponse<T>(val type: String, val id: Long, val data: List<T>)
