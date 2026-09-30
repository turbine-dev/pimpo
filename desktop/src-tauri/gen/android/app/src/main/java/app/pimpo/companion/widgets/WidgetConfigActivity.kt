package app.pimpo.companion.widgets

import android.app.Activity
import android.appwidget.AppWidgetManager
import android.content.Intent
import android.os.Bundle
import android.view.Gravity
import android.widget.ArrayAdapter
import android.widget.LinearLayout
import android.widget.ListView
import android.widget.TextView
import app.pimpo.companion.R

// WidgetConfigActivity asks which Pimpo widget a new home-screen widget
// shows, when it is added from the launcher rather than from the app: one
// of the widgets pinned to this phone.
class WidgetConfigActivity : Activity() {
  private var appWidgetId = AppWidgetManager.INVALID_APPWIDGET_ID

  override fun onCreate(savedInstanceState: Bundle?) {
    super.onCreate(savedInstanceState)
    setResult(RESULT_CANCELED)
    appWidgetId = intent?.extras?.getInt(AppWidgetManager.EXTRA_APPWIDGET_ID, AppWidgetManager.INVALID_APPWIDGET_ID) ?: AppWidgetManager.INVALID_APPWIDGET_ID
    if (appWidgetId == AppWidgetManager.INVALID_APPWIDGET_ID) return finish()
    val box = LinearLayout(this)
    box.orientation = LinearLayout.VERTICAL
    val pad = (20 * resources.displayMetrics.density).toInt()
    box.setPadding(pad, pad, pad, pad)
    val heading = TextView(this)
    heading.text = getString(R.string.widget_choose)
    heading.textSize = 20f
    box.addView(heading)
    val note = TextView(this)
    note.setPadding(0, pad / 2, 0, pad / 2)
    box.addView(note)
    val list = ListView(this)
    box.addView(list)
    setContentView(box)
    note.text = getString(R.string.widget_loading)
    Thread {
      WidgetFeed.fetch(this)
      val widgets = WidgetStore.widgets(this)
      runOnUiThread {
        if (widgets.isEmpty()) {
          note.text = getString(R.string.widget_none)
          note.gravity = Gravity.START
          return@runOnUiThread
        }
        note.text = ""
        val names = widgets.map { w -> w.optString("title", w.optString("id")) }
        list.adapter = ArrayAdapter(this, android.R.layout.simple_list_item_1, names)
        list.setOnItemClickListener { _, _, pos, _ ->
          WidgetStore.assign(this, appWidgetId, widgets[pos].optString("id"))
          PimpoWidgetProvider.redraw(this, AppWidgetManager.getInstance(this), appWidgetId)
          setResult(RESULT_OK, Intent().putExtra(AppWidgetManager.EXTRA_APPWIDGET_ID, appWidgetId))
          finish()
        }
      }
    }.start()
  }
}
