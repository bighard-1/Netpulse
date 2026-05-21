package com.netpulse.mobile.util

import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import kotlin.math.abs

private val timeFmt = DateTimeFormatter.ofPattern("HH:mm").withZone(ZoneId.systemDefault())
private val dateFmt = DateTimeFormatter.ofPattern("MM-dd HH:mm").withZone(ZoneId.systemDefault())

fun formatTime(raw: String): String = runCatching {
    val i = Instant.parse(raw)
    val age = abs(Instant.now().epochSecond - i.epochSecond)
    if (age > 36 * 3600) dateFmt.format(i) else timeFmt.format(i)
}.getOrDefault(raw.take(16))

fun formatBps(v: Double?): String {
    var n = v ?: 0.0
    val units = listOf("bps", "Kbps", "Mbps", "Gbps", "Tbps")
    var idx = 0
    while (n >= 1000.0 && idx < units.lastIndex) {
        n /= 1000.0
        idx++
    }
    val digits = if (n >= 100) 0 else if (n >= 10) 1 else 2
    return "%.${digits}f %s".format(n, units[idx])
}

fun displayPortName(name: String, customName: String?, remark: String?): String {
    val suffix = customName?.takeIf { it.isNotBlank() } ?: remark?.takeIf { it.isNotBlank() }
    return if (suffix.isNullOrBlank() || name.contains(suffix)) name else "$name $suffix"
}
