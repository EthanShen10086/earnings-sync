package com.earningssync.mobile

import okhttp3.OkHttpClient
import okhttp3.Request
import org.json.JSONObject
import java.util.concurrent.TimeUnit

data class FactRow(
    val fiscalYear: Int,
    val fiscalQuarter: Int,
    val lineItem: String,
    val value: Double?,
    val unit: String,
)

data class MetaSnapshot(
    val apiVersion: String,
    val wantMode: String,
    val mode: String,
    val sheetsKind: String,
    val pgKind: String,
) {
    fun storageLine(): String =
        "want=$wantMode · effective=$mode · sheets=$sheetsKind · pg=$pgKind"
}

class ApiClient(
    baseURL: String,
    private val http: OkHttpClient = OkHttpClient.Builder()
        .connectTimeout(10, TimeUnit.SECONDS)
        .readTimeout(20, TimeUnit.SECONDS)
        .build(),
) {
    private val root = baseURL.trimEnd('/')

    fun meta(): MetaSnapshot {
        val body = get("/v1/meta")
        val storage = body.optJSONObject("storage") ?: JSONObject()
        return MetaSnapshot(
            apiVersion = body.optString("api_version", ""),
            wantMode = storage.optString("want_mode", "unknown"),
            mode = storage.optString("mode", "unknown"),
            sheetsKind = storage.optString("sheets_kind", "unknown"),
            pgKind = storage.optString("pg_kind", "unknown"),
        )
    }

    fun facts(ticker: String): List<FactRow> {
        val body = get("/v1/facts?ticker=${ticker.trim().uppercase()}")
        val arr = body.optJSONArray("facts") ?: return emptyList()
        val out = ArrayList<FactRow>(arr.length())
        for (i in 0 until arr.length()) {
            val o = arr.getJSONObject(i)
            val value = if (o.isNull("value")) null else o.optDouble("value")
            out.add(
                FactRow(
                    fiscalYear = o.optInt("fiscal_year"),
                    fiscalQuarter = o.optInt("fiscal_quarter"),
                    lineItem = o.optString("line_item"),
                    value = value,
                    unit = o.optString("unit"),
                ),
            )
        }
        return out
    }

    private fun get(path: String): JSONObject {
        val req = Request.Builder()
            .url(root + path)
            .header("Accept", "application/json")
            .get()
            .build()
        http.newCall(req).execute().use { resp ->
            val text = resp.body?.string().orEmpty()
            if (!resp.isSuccessful) {
                throw IllegalStateException("HTTP ${resp.code}: $text")
            }
            return JSONObject(text)
        }
    }
}
