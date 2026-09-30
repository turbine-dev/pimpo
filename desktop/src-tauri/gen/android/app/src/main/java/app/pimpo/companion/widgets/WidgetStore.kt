package app.pimpo.companion.widgets

import android.content.Context
import android.content.SharedPreferences
import org.json.JSONArray
import org.json.JSONObject

// WidgetStore keeps, in the app's private storage, the Pimpo the phone's
// widgets read from, the phone's widget key, the last feed, and which
// Pimpo widget each home-screen widget shows. The key only reads the
// widgets pinned to this phone.
object WidgetStore {
  private fun prefs(c: Context): SharedPreferences = c.getSharedPreferences("pimpo.widgets", Context.MODE_PRIVATE)

  fun setup(c: Context, base: String, key: String) {
    prefs(c).edit().putString("base", base).putString("key", key).apply()
  }

  fun base(c: Context): String? = prefs(c).getString("base", null)
  fun key(c: Context): String? = prefs(c).getString("key", null)

  fun saveFeed(c: Context, json: String) {
    prefs(c).edit().putString("feed", json).putLong("fetched", System.currentTimeMillis()).apply()
  }

  fun fetched(c: Context): Long = prefs(c).getLong("fetched", 0)

  /** The widgets of the last feed, by id. */
  fun widgets(c: Context): List<JSONObject> {
    val raw = prefs(c).getString("feed", null) ?: return emptyList()
    return try {
      val list: JSONArray = JSONObject(raw).optJSONArray("widgets") ?: JSONArray()
      (0 until list.length()).map { list.getJSONObject(it) }
    } catch (e: Exception) {
      emptyList()
    }
  }

  fun widget(c: Context, id: String): JSONObject? = widgets(c).firstOrNull { it.optString("id") == id }

  fun assign(c: Context, appWidgetId: Int, pimpoId: String) {
    prefs(c).edit().putString("w.$appWidgetId", pimpoId).apply()
  }

  fun assigned(c: Context, appWidgetId: Int): String? = prefs(c).getString("w.$appWidgetId", null)

  fun forget(c: Context, appWidgetId: Int) {
    prefs(c).edit().remove("w.$appWidgetId").apply()
  }

  fun setPending(c: Context, pimpoId: String) {
    prefs(c).edit().putString("pending", pimpoId).apply()
  }

  fun takePending(c: Context): String? {
    val p = prefs(c).getString("pending", null)
    prefs(c).edit().remove("pending").apply()
    return p
  }
}
