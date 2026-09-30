package app.pimpo.companion.widgets

import android.app.PendingIntent
import android.appwidget.AppWidgetManager
import android.appwidget.AppWidgetProvider
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import app.pimpo.companion.MainActivity
import app.pimpo.companion.R

// PimpoWidgetProvider is the home-screen widget. The system wakes it about
// every half hour, and the app wakes it when it opens; each time it fetches
// the feed and redraws every Pimpo widget on the home screen. Tapping one
// opens the app.
class PimpoWidgetProvider : AppWidgetProvider() {
  companion object {
    const val PINNED = "app.pimpo.companion.widgets.PINNED"

    fun redrawAll(c: Context) {
      val m = AppWidgetManager.getInstance(c)
      val ids = m.getAppWidgetIds(ComponentName(c, PimpoWidgetProvider::class.java))
      for (id in ids) redraw(c, m, id)
    }

    fun redraw(c: Context, m: AppWidgetManager, appWidgetId: Int) {
      val pimpoId = WidgetStore.assigned(c, appWidgetId)
      val views = WidgetRender.views(c, pimpoId?.let { WidgetStore.widget(c, it) }, WidgetStore.fetched(c))
      val open = Intent(c, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
      views.setOnClickPendingIntent(R.id.root, PendingIntent.getActivity(c, 0, open, PendingIntent.FLAG_IMMUTABLE))
      m.updateAppWidget(appWidgetId, views)
    }

    /** refresh fetches the feed off the main thread, then redraws. */
    fun refresh(c: Context, done: (() -> Unit)? = null) {
      val app = c.applicationContext
      Thread {
        WidgetFeed.fetch(app)
        redrawAll(app)
        done?.invoke()
      }.start()
    }
  }

  override fun onUpdate(c: Context, m: AppWidgetManager, ids: IntArray) {
    for (id in ids) redraw(c, m, id)
    val pending = goAsync()
    refresh(c) { pending.finish() }
  }

  override fun onReceive(c: Context, intent: Intent) {
    if (intent.action == PINNED) {
      val id = intent.getIntExtra(AppWidgetManager.EXTRA_APPWIDGET_ID, AppWidgetManager.INVALID_APPWIDGET_ID)
      val pimpoId = WidgetStore.takePending(c)
      if (id != AppWidgetManager.INVALID_APPWIDGET_ID && pimpoId != null) {
        WidgetStore.assign(c, id, pimpoId)
        redraw(c, AppWidgetManager.getInstance(c), id)
      }
      return
    }
    super.onReceive(c, intent)
  }

  override fun onDeleted(c: Context, ids: IntArray) {
    for (id in ids) WidgetStore.forget(c, id)
  }
}
