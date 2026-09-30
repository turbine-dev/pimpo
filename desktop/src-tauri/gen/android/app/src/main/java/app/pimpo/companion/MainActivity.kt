package app.pimpo.companion

import android.os.Bundle
import android.webkit.WebView
import androidx.activity.enableEdgeToEdge
import app.pimpo.companion.widgets.PimpoWidgetProvider
import app.pimpo.companion.widgets.WidgetBridge

class MainActivity : TauriActivity() {
  override fun onCreate(savedInstanceState: Bundle?) {
    enableEdgeToEdge()
    super.onCreate(savedInstanceState)
  }

  // Pimpo's page puts widgets on the home screen through this bridge.
  override fun onWebViewCreate(webView: WebView) {
    webView.addJavascriptInterface(WidgetBridge(applicationContext, webView), "PimpoWidgets")
  }

  // Opening the app refreshes its home-screen widgets.
  override fun onResume() {
    super.onResume()
    PimpoWidgetProvider.refresh(this)
  }
}
