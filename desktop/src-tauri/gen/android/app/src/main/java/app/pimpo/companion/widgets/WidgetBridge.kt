package app.pimpo.companion.widgets

import android.app.PendingIntent
import android.appwidget.AppWidgetManager
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.net.Uri
import android.os.Build
import android.os.Handler
import android.os.Looper
import android.webkit.JavascriptInterface
import android.webkit.WebView
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit

// WidgetBridge lets Pimpo's page in the app put a widget on the home screen.
// The page hands over the widget key the server just made for this phone;
// the address the widgets read from is the page's own, never one the page
// names, and only http(s).
class WidgetBridge(private val context: Context, private val webView: WebView) {
  private fun origin(): String? {
    val latch = CountDownLatch(1)
    var url: String? = null
    Handler(Looper.getMainLooper()).post {
      url = webView.url
      latch.countDown()
    }
    latch.await(2, TimeUnit.SECONDS)
    val u = Uri.parse(url ?: return null)
    if (u.scheme != "https" && u.scheme != "http") return null
    val host = u.host ?: return null
    val port = if (u.port > 0) ":${u.port}" else ""
    return "${u.scheme}://$host$port"
  }

  @JavascriptInterface
  fun available(): Boolean =
    Build.VERSION.SDK_INT >= Build.VERSION_CODES.O && AppWidgetManager.getInstance(context).isRequestPinAppWidgetSupported

  /** setup keeps the key; the page calls it after every pin or unpin. */
  @JavascriptInterface
  fun setup(key: String): Boolean {
    if (!key.startsWith("wk_") || key.length > 80) return false
    val base = origin() ?: return false
    WidgetStore.setup(context, base, key)
    Thread { WidgetFeed.fetch(context); PimpoWidgetProvider.redrawAll(context) }.start()
    return true
  }

  /** pin asks the launcher to place a widget showing the Pimpo widget id. */
  @JavascriptInterface
  fun pin(id: String): Boolean {
    if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O || id.length > 120) return false
    val manager = AppWidgetManager.getInstance(context)
    if (!manager.isRequestPinAppWidgetSupported) return false
    WidgetStore.setPending(context, id)
    val done = Intent(context, PimpoWidgetProvider::class.java).setAction(PimpoWidgetProvider.PINNED)
    val flags = PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_MUTABLE
    val callback = PendingIntent.getBroadcast(context, 0, done, flags)
    return manager.requestPinAppWidget(ComponentName(context, PimpoWidgetProvider::class.java), null, callback)
  }
}
