package app.pimpo.companion.widgets

import android.content.Context
import android.graphics.Bitmap
import android.graphics.Canvas
import android.graphics.Color
import android.graphics.LinearGradient
import android.graphics.Paint
import android.graphics.Path
import android.graphics.RectF
import android.graphics.Shader
import android.text.format.DateUtils
import android.view.View
import android.widget.RemoteViews
import app.pimpo.companion.R
import org.json.JSONArray
import org.json.JSONObject
import java.text.NumberFormat
import java.util.Currency
import java.util.Locale
import kotlin.math.abs
import kotlin.math.max
import kotlin.math.min

// WidgetRender draws one Pimpo widget into a home-screen widget: its title,
// the main value or text, a few lines of detail and, for numbers and
// charts, a small chart. Everything is drawn as text, as in the app.
object WidgetRender {
  private val palette = intArrayOf(0xFF4F7DF3.toInt(), 0xFF2DB37A.toInt(), 0xFFE8A33D.toInt(), 0xFFD9577B.toInt(), 0xFF8A6CE0.toInt())

  fun views(c: Context, w: JSONObject?, fetched: Long): RemoteViews {
    val v = RemoteViews(c.packageName, R.layout.pimpo_widget)
    if (w == null) {
      v.setTextViewText(R.id.title, "Pimpo")
      v.setTextViewText(R.id.value, "")
      v.setTextViewText(R.id.detail, c.getString(R.string.widget_missing))
      v.setViewVisibility(R.id.chart, View.GONE)
      v.setTextViewText(R.id.updated, "")
      return v
    }
    val s = w.optJSONObject("snapshot") ?: JSONObject()
    v.setTextViewText(R.id.title, title(c, w))
    var value = ""
    val lines = mutableListOf<String>()
    var chart: Bitmap? = null
    when (w.optString("kind")) {
      "metric" -> {
        value = number(s, "value")
        if (s.has("trend")) {
          val t = s.optDouble("trend")
          lines += (if (t >= 0) "▲ " else "▼ ") + String.format(Locale.getDefault(), "%.1f%%", abs(t))
        }
        sub(s)?.let { lines += it }
        val history = w.optJSONArray("history")
        if (history != null && history.length() > 1) chart = line((0 until history.length()).map { history.getJSONObject(it).optDouble("v") }, palette[0])
      }
      "progress" -> {
        val goal = s.optDouble("goal", 0.0)
        val cur = s.optDouble("value", 0.0)
        val pct = if (goal > 0) (cur / goal * 100).toInt() else 0
        value = "$pct%"
        lines += number(s, "value") + " / " + format(goal, s.optString("unit"))
        chart = bar(min(max(cur / max(goal, 1e-9), 0.0), 1.0))
      }
      "status" -> {
        value = when (s.optString("status")) { "alert" -> "● " + c.getString(R.string.status_alert); "warn" -> "● " + c.getString(R.string.status_warn); else -> "● " + c.getString(R.string.status_ok) }
        sub(s)?.let { lines += it }
        s.optString("text").takeIf { it.isNotEmpty() }?.let { lines += it }
        items(s.optJSONArray("items"), lines)
      }
      "list" -> items(s.optJSONArray("items"), lines)
      "table" -> {
        val cols = s.optJSONArray("columns")
        if (cols != null && cols.length() > 0) lines += (0 until cols.length()).joinToString("  ·  ") { cols.optString(it) }
        val rows = s.optJSONArray("rows") ?: JSONArray()
        for (i in 0 until min(rows.length(), 5)) {
          val r = rows.optJSONArray(i) ?: continue
          lines += (0 until r.length()).joinToString("  ·  ") { r.optString(it) }
        }
      }
      "chart" -> {
        val series = s.optJSONArray("series") ?: JSONArray()
        val first = series.optJSONObject(0)?.optJSONArray("points") ?: JSONArray()
        val ys = (0 until first.length()).map { first.getJSONObject(it).optDouble("y") }
        if (ys.isNotEmpty()) {
          value = format(ys.last(), s.optString("unit"))
          chart = if (s.optString("chart") == "line" || s.optString("chart") == "area") line(ys, palette[0]) else bars(ys)
          if (s.optString("chart") == "donut") {
            val total = ys.sum()
            for (i in 0 until min(first.length(), 4)) {
              val p = first.getJSONObject(i)
              lines += p.optString("label") + "  " + (if (total > 0) (p.optDouble("y") / total * 100).toInt() else 0) + "%"
            }
            value = ""
          }
        }
      }
      else -> {
        val text = s.optString("text")
        lines += if (text == "-" || text.isEmpty()) sub(s) ?: "" else text
      }
    }
    v.setTextViewText(R.id.value, value)
    v.setViewVisibility(R.id.value, if (value.isEmpty()) View.GONE else View.VISIBLE)
    v.setTextViewText(R.id.detail, lines.filter { it.isNotBlank() }.take(6).joinToString("\n"))
    if (chart != null) {
      v.setImageViewBitmap(R.id.chart, chart)
      v.setViewVisibility(R.id.chart, View.VISIBLE)
    } else {
      v.setViewVisibility(R.id.chart, View.GONE)
    }
    val at = if (fetched > 0) DateUtils.getRelativeTimeSpanString(fetched, System.currentTimeMillis(), DateUtils.MINUTE_IN_MILLIS).toString() else ""
    v.setTextViewText(R.id.updated, at)
    return v
  }

  private fun title(c: Context, w: JSONObject): String = when (w.optString("id")) {
    "builtin:needs" -> c.getString(R.string.builtin_needs)
    "builtin:today" -> c.getString(R.string.builtin_today)
    "builtin:spent" -> c.getString(R.string.builtin_spent)
    "builtin:budget" -> c.getString(R.string.builtin_budget)
    "builtin:reminders" -> c.getString(R.string.builtin_reminders)
    else -> w.optString("title", "Pimpo")
  }

  private fun sub(s: JSONObject): String? = s.optString("subtitle").takeIf { it.isNotEmpty() }

  private fun items(list: JSONArray?, lines: MutableList<String>) {
    if (list == null) return
    for (i in 0 until min(list.length(), 5)) {
      val it = list.optJSONObject(i) ?: continue
      val extra = listOf(it.optString("value"), it.optString("badge")).filter { x -> x.isNotEmpty() }.map { x -> clock(x) }.joinToString(" · ")
      lines += "• " + it.optString("title") + if (extra.isNotEmpty()) "  —  $extra" else ""
    }
  }

  private val iso = Regex("""^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}""")

  /** clock shows a time the server sent as RFC 3339 in the phone's own way. */
  private fun clock(x: String): String {
    if (!iso.containsMatchIn(x)) return x
    return try {
      val plain = x.replace(Regex("""\.\d+"""), "").replace(Regex("""Z$"""), "+00:00")
      val t = java.text.SimpleDateFormat("yyyy-MM-dd'T'HH:mm:ssXXX", Locale.US).parse(plain) ?: return x
      java.text.DateFormat.getTimeInstance(java.text.DateFormat.SHORT).format(t)
    } catch (e: Exception) {
      x
    }
  }

  private fun number(s: JSONObject, field: String): String = if (s.has(field)) format(s.optDouble(field), s.optString("unit")) else "—"

  fun format(x: Double, unit: String): String {
    if (unit.length == 3 && unit.all { it.isUpperCase() }) {
      try {
        val f = NumberFormat.getCurrencyInstance()
        f.currency = Currency.getInstance(unit)
        return f.format(x)
      } catch (e: Exception) { }
    }
    val n = NumberFormat.getNumberInstance()
    n.maximumFractionDigits = if (abs(x) >= 100) 0 else 2
    return when (unit) {
      "" -> n.format(x)
      "%" -> n.format(x) + "%"
      else -> n.format(x) + " " + unit
    }
  }

  private const val W = 600
  private const val H = 150

  private fun line(ys: List<Double>, color: Int): Bitmap {
    val bmp = Bitmap.createBitmap(W, H, Bitmap.Config.ARGB_8888)
    val cv = Canvas(bmp)
    val lo = ys.minOrNull() ?: 0.0
    val hi = ys.maxOrNull() ?: 1.0
    val span = if (hi - lo == 0.0) 1.0 else hi - lo
    val pad = 8f
    val pts = ys.mapIndexed { i, y ->
      val x = pad + (W - 2 * pad) * (if (ys.size == 1) 0.5f else i.toFloat() / (ys.size - 1))
      val py = H - pad - (H - 2 * pad) * ((y - lo) / span).toFloat()
      x to py
    }
    val path = Path()
    pts.forEachIndexed { i, (x, y) -> if (i == 0) path.moveTo(x, y) else path.lineTo(x, y) }
    val fill = Path(path)
    fill.lineTo(pts.last().first, H.toFloat())
    fill.lineTo(pts.first().first, H.toFloat())
    fill.close()
    val area = Paint(Paint.ANTI_ALIAS_FLAG)
    area.shader = LinearGradient(0f, 0f, 0f, H.toFloat(), (color and 0x00FFFFFF) or 0x55000000, color and 0x00FFFFFF, Shader.TileMode.CLAMP)
    cv.drawPath(fill, area)
    val stroke = Paint(Paint.ANTI_ALIAS_FLAG)
    stroke.color = color
    stroke.style = Paint.Style.STROKE
    stroke.strokeWidth = 5f
    stroke.strokeJoin = Paint.Join.ROUND
    stroke.strokeCap = Paint.Cap.ROUND
    cv.drawPath(path, stroke)
    val dot = Paint(Paint.ANTI_ALIAS_FLAG)
    dot.color = color
    cv.drawCircle(pts.last().first, pts.last().second, 7f, dot)
    return bmp
  }

  private fun bars(ys: List<Double>): Bitmap {
    val bmp = Bitmap.createBitmap(W, H, Bitmap.Config.ARGB_8888)
    val cv = Canvas(bmp)
    val hi = max(ys.maxOrNull() ?: 1.0, 1e-9)
    val n = min(ys.size, 24)
    val shown = ys.takeLast(n)
    val gap = 8f
    val bw = (W - gap * (n - 1)) / n
    val p = Paint(Paint.ANTI_ALIAS_FLAG)
    shown.forEachIndexed { i, y ->
      p.color = palette[0]
      val h = max(4f, (H - 4) * (max(y, 0.0) / hi).toFloat())
      val x = i * (bw + gap)
      cv.drawRoundRect(RectF(x, H - h, x + bw, H.toFloat()), 6f, 6f, p)
    }
    return bmp
  }

  private fun bar(frac: Double): Bitmap {
    val bmp = Bitmap.createBitmap(W, 40, Bitmap.Config.ARGB_8888)
    val cv = Canvas(bmp)
    val bg = Paint(Paint.ANTI_ALIAS_FLAG)
    bg.color = Color.argb(40, 128, 128, 128)
    cv.drawRoundRect(RectF(0f, 8f, W.toFloat(), 32f), 12f, 12f, bg)
    if (frac > 0) {
      val fg = Paint(Paint.ANTI_ALIAS_FLAG)
      fg.color = palette[1]
      cv.drawRoundRect(RectF(0f, 8f, (W * frac).toFloat(), 32f), 12f, 12f, fg)
    }
    return bmp
  }
}
