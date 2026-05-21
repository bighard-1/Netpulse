package com.netpulse.mobile.data

import android.content.Context
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey

class SessionStore(context: Context) {
    private val prefs = try {
        val key = MasterKey.Builder(context).setKeyScheme(MasterKey.KeyScheme.AES256_GCM).build()
        EncryptedSharedPreferences.create(
            context,
            "netpulse_secure_session",
            key,
            EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
            EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM
        )
    } catch (_: Throwable) {
        context.getSharedPreferences("netpulse_session_fallback", Context.MODE_PRIVATE)
    }

    var baseUrl: String
        get() = prefs.getString("base_url", "http://119.40.55.18:18080/api") ?: "http://119.40.55.18:18080/api"
        set(value) = prefs.edit().putString("base_url", value.trim().trimEnd('/')).apply()

    var token: String
        get() = prefs.getString("token", "") ?: ""
        set(value) = prefs.edit().putString("token", value).apply()

    fun clear() {
        prefs.edit().remove("token").apply()
    }
}
