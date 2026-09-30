package app.pimpo.companion.widgets

import android.content.Context
import java.net.HttpURLConnection
import java.net.URL

// WidgetFeed fetches the latest snapshots of the widgets pinned to this
// phone. Away from home, or with Pimpo off, the last ones stay, with their
// time.
object WidgetFeed {
  /** fetch returns whether a fresh feed arrived. Call it off the main thread. */
  fun fetch(c: Context): Boolean {
    val base = WidgetStore.base(c) ?: return false
    val key = WidgetStore.key(c) ?: return false
    return try {
      val conn = URL("$base/api/widgets/feed").openConnection() as HttpURLConnection
      conn.connectTimeout = 8000
      conn.readTimeout = 10000
      conn.setRequestProperty("Authorization", "Bearer $key")
      conn.setRequestProperty("Accept", "application/json")
      try {
        if (conn.responseCode != 200) return false
        val body = conn.inputStream.bufferedReader().use { it.readText() }
        WidgetStore.saveFeed(c, body)
        true
      } finally {
        conn.disconnect()
      }
    } catch (e: Exception) {
      false
    }
  }
}
