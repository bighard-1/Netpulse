package com.netpulse.mobile

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContent {
            MaterialTheme {
                Surface(Modifier.fillMaxSize()) {
                    val sample = listOf(
                        Triple("192.168.1.1", "Huawei", "Core switch"),
                        Triple("192.168.1.2", "H3C", "Access switch")
                    )
                    Column(Modifier.fillMaxSize().padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                        Text("NetPulse Mobile", style = MaterialTheme.typography.headlineSmall)
                        Surface(color = MaterialTheme.colorScheme.surfaceVariant, tonalElevation = 2.dp) {
                            Row(Modifier.fillMaxWidth().padding(12.dp), horizontalArrangement = Arrangement.SpaceBetween) {
                                Text("Total: ${sample.size}")
                                Text("Online: 1", color = Color(0xFF2E7D32))
                                Text("Offline: 1", color = Color(0xFFC62828))
                            }
                        }
                        LazyColumn(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                            items(sample) { d ->
                                ElevatedCard(Modifier.fillMaxWidth()) {
                                    Column(Modifier.padding(12.dp)) {
                                        Text(d.first, style = MaterialTheme.typography.titleMedium)
                                        Text("${d.second} · ${d.third}")
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }
    }
}
