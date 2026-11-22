package com.earningssync.mobile

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContent {
            MaterialTheme {
                Surface(modifier = Modifier.fillMaxSize()) {
                    var baseUrl by remember { mutableStateOf("http://10.0.2.2:8090") }
                    var ticker by remember { mutableStateOf("AAPL") }
                    var status by remember { mutableStateOf("尚未加载") }
                    var storage by remember { mutableStateOf("") }
                    var facts by remember { mutableStateOf(listOf<FactRow>()) }
                    val scope = rememberCoroutineScope()

                    Column(
                        modifier = Modifier.padding(16.dp),
                        verticalArrangement = Arrangement.spacedBy(12.dp),
                    ) {
                        Text("earnings-sync", style = MaterialTheme.typography.headlineSmall)
                        OutlinedTextField(
                            value = baseUrl,
                            onValueChange = { baseUrl = it },
                            label = { Text("API Base") },
                            modifier = Modifier.fillMaxWidth(),
                        )
                        OutlinedTextField(
                            value = ticker,
                            onValueChange = { ticker = it },
                            label = { Text("Ticker") },
                            modifier = Modifier.fillMaxWidth(),
                        )
                        Button(onClick = {
                            scope.launch {
                                status = "加载中…"
                                try {
                                    val client = ApiClient(baseUrl)
                                    val meta = withContext(Dispatchers.IO) { client.meta() }
                                    storage = meta.storageLine()
                                    val out = withContext(Dispatchers.IO) { client.facts(ticker) }
                                    facts = out
                                    status = if (out.isEmpty()) {
                                        "无数据（先 dual/pg-only ingest）"
                                    } else {
                                        "共 ${out.size} 条 · API ${meta.apiVersion}"
                                    }
                                } catch (e: Exception) {
                                    facts = emptyList()
                                    status = e.message ?: e.toString()
                                }
                            }
                        }) {
                            Text("加载事实")
                        }
                        Text(status, style = MaterialTheme.typography.bodyMedium)
                        if (storage.isNotEmpty()) {
                            Text(storage, style = MaterialTheme.typography.bodySmall)
                        }
                        LazyColumn(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                            items(facts) { row ->
                                Text(
                                    "${row.fiscalYear}Q${row.fiscalQuarter} · ${row.lineItem} = ${row.value ?: "—"} ${row.unit}",
                                    style = MaterialTheme.typography.bodyMedium,
                                )
                            }
                        }
                    }
                }
            }
        }
    }
}
