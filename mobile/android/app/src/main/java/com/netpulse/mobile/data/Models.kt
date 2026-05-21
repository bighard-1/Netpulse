package com.netpulse.mobile.data

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

@Serializable
data class LoginRequest(val username: String, val password: String)

@Serializable
data class LoginResponse(val token: String, val user: UserInfo = UserInfo())

@Serializable
data class UserInfo(val username: String = "", val role: String = "")

@Serializable
data class Device(
    val id: Long,
    val ip: String = "",
    val name: String = "",
    val brand: String = "",
    val remark: String = "",
    val status: String = "unknown",
    val interfaces: List<Port> = emptyList()
)

@Serializable
data class Port(
    val id: Long,
    val index: Int = 0,
    val name: String = "",
    @SerialName("custom_name") val customName: String? = null,
    val remark: String = "",
    @SerialName("speed_mbps") val speedMbps: Int? = null,
    @SerialName("oper_status") val operStatus: Int? = null,
    @SerialName("admin_status") val adminStatus: Int? = null,
    @SerialName("traffic_in_bps") val trafficInBps: Long? = null,
    @SerialName("traffic_out_bps") val trafficOutBps: Long? = null,
    @SerialName("device_id") val deviceId: Long? = null,
    @SerialName("device_name") val deviceName: String? = null,
    @SerialName("device_ip") val deviceIp: String? = null
)

@Serializable
data class DeviceHistoryPoint(
    val timestamp: String,
    @SerialName("cpu_usage") val cpuUsage: Double? = null,
    @SerialName("mem_usage") val memUsage: Double? = null
)

@Serializable
data class TrafficHistoryPoint(
    val timestamp: String,
    @SerialName("traffic_in_bps") val trafficInBps: Double? = null,
    @SerialName("traffic_out_bps") val trafficOutBps: Double? = null
)

@Serializable
data class HistoryResponse<T>(
    val type: String = "",
    val id: Long = 0,
    val interval: String? = null,
    @SerialName("sampled_interval") val sampledInterval: String? = null,
    @SerialName("source_table") val sourceTable: String? = null,
    val data: List<T> = emptyList()
)

@Serializable
data class SearchItem(
    val type: String = "",
    @SerialName("device_id") val deviceId: Long? = null,
    @SerialName("interface_id") val interfaceId: Long? = null,
    @SerialName("device_name") val deviceName: String? = null,
    @SerialName("device_ip") val deviceIp: String? = null,
    @SerialName("interface_name") val interfaceName: String? = null,
    @SerialName("interface_custom_name") val interfaceCustomName: String? = null,
    @SerialName("interface_remark") val interfaceRemark: String? = null,
    @SerialName("match_field") val matchField: String? = null,
    val snippet: String? = null
)
