package com.icegood.medialibrary;

import android.content.Intent;
import android.content.SharedPreferences;
import android.graphics.Color;
import android.net.Uri;
import android.os.Build;
import android.os.Bundle;
import android.view.View;
import android.view.WindowManager;
import android.view.autofill.AutofillManager;
import android.webkit.CookieManager;
import android.webkit.JavascriptInterface;
import android.webkit.WebSettings;
import android.webkit.WebView;

import androidx.core.content.FileProvider;

import java.io.File;
import java.io.FileOutputStream;
import java.io.InputStream;
import java.net.HttpURLConnection;
import java.net.URL;

import androidx.core.view.DisplayCutoutCompat;
import androidx.core.view.ViewCompat;
import androidx.core.view.WindowCompat;
import androidx.core.view.WindowInsetsCompat;
import androidx.core.view.WindowInsetsControllerCompat;

import com.getcapacitor.BridgeActivity;

public class MainActivity extends BridgeActivity {

    private volatile int safeTop = 0;
    private volatile int safeBottom = 0;
    private volatile int safeLeft = 0;
    private volatile int safeRight = 0;

    @Override
    public void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        // The app boots from the Capacitor origin (https://localhost) and then
        // fetches from the user's self-hosted server, which may be plain HTTP
        // (e.g. LAN-only installs). Without this, those http fetches are treated
        // as mixed content and blocked, so the server-address form fails while
        // the same URL works fine in the system browser.
        final WebView webView = getBridge() != null ? getBridge().getWebView() : null;
        if (webView == null) {
            return;
        }
        webView.getSettings().setMixedContentMode(WebSettings.MIXED_CONTENT_ALWAYS_ALLOW);
        // Password managers (Bitwarden, Google, etc.) offer their overlay on
        // WebView inputs only when the WebView explicitly opts in to the
        // Android autofill framework. Capacitor keeps the default AUTO, which
        // some OEMs treat as disabled inside WebViews.
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            webView.setImportantForAutofill(View.IMPORTANT_FOR_AUTOFILL_YES);
        }
        WebView.setWebContentsDebuggingEnabled(true);

        // Android 15 (targetSdk 35) enforces edge-to-edge: the WebView draws
        // under the status and navigation bars. The user wants the image/video
        // to keep the whole screen, so the app chrome (top menu bar, viewer
        // buttons, video controls) must move away from the system bars. Expose
        // the real bar sizes to the SPA as a JS interface (browser-only pages
        // never see it, so their layout is untouched) and enable the same
        // full-bleed on pre-15 devices for consistent behaviour.
        WindowCompat.setDecorFitsSystemWindows(getWindow(), false);
        getWindow().setStatusBarColor(Color.TRANSPARENT);
        getWindow().setNavigationBarColor(Color.TRANSPARENT);
        // Let content (video/image fullscreen) reach the display cutout too;
        // the SPA shifts its own controls inward via the inset variables, so
        // the punch-hole never hides a tappable element.
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
            getWindow().getAttributes().layoutInDisplayCutoutMode =
                WindowManager.LayoutParams.LAYOUT_IN_DISPLAY_CUTOUT_MODE_ALWAYS;
        } else if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
            getWindow().getAttributes().layoutInDisplayCutoutMode =
                WindowManager.LayoutParams.LAYOUT_IN_DISPLAY_CUTOUT_MODE_SHORT_EDGES;
        }
        webView.addJavascriptInterface(new Object() {
            @JavascriptInterface
            public int getTop() {
                return safeTop;
            }

            @JavascriptInterface
            public int getBottom() {
                return safeBottom;
            }

            @JavascriptInterface
            public int getLeft() {
                return safeLeft;
            }

            @JavascriptInterface
            public int getRight() {
                return safeRight;
            }

            // The page background (light/dark theme) sits behind the system
            // bars, so the bar icons must be inverted to stay readable.
            @JavascriptInterface
            public void setDarkSystemIcons(final boolean dark) {
                runOnUiThread(() -> {
                    final View decor = getWindow().getDecorView();
                    int flags = decor.getSystemUiVisibility();
                    if (dark) {
                        flags |= View.SYSTEM_UI_FLAG_LIGHT_STATUS_BAR | View.SYSTEM_UI_FLAG_LIGHT_NAVIGATION_BAR;
                    } else {
                        flags &= ~(View.SYSTEM_UI_FLAG_LIGHT_STATUS_BAR | View.SYSTEM_UI_FLAG_LIGHT_NAVIGATION_BAR);
                    }
                    decor.setSystemUiVisibility(flags);
                });
            }

            // In-app fullscreen: the WebView cancels DOM requestFullscreen, so
            // the SPA asks the activity to hide/restore the system bars while
            // the media is pinned edge-to-edge (see Viewer.toggleFullscreen).
            @JavascriptInterface
            public void setImmersive(final boolean immersive) {
                runOnUiThread(() -> {
                    WindowInsetsControllerCompat controller =
                        ViewCompat.getWindowInsetsController(getWindow().getDecorView());
                    if (controller == null) {
                        return;
                    }
                    if (immersive) {
                        controller.setSystemBarsBehavior(
                            WindowInsetsControllerCompat.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE);
                        controller.hide(WindowInsetsCompat.Type.systemBars());
                    } else {
                        controller.show(WindowInsetsCompat.Type.systemBars());
                    }
                });
            }

            // Downloads a document (PDF, etc.) with the WebView's auth cookie,
            // writes it to the app cache, and opens it via a native viewer intent.
            @JavascriptInterface
            public void openDocument(final String url) {
                new Thread(() -> {
                    try {
                        HttpURLConnection conn = (HttpURLConnection) new URL(url).openConnection();
                        String cookies = CookieManager.getInstance().getCookie(url);
                        if (cookies != null) {
                            conn.setRequestProperty("Cookie", cookies);
                        }
                        conn.connect();
                        if (conn.getResponseCode() != 200) {
                            android.util.Log.e("MLDocument",
                                "download failed: HTTP " + conn.getResponseCode() + " for " + url);
                            return;
                        }
                        String ct = conn.getContentType();
                        String ext = ".pdf";
                        if (ct != null && ct.contains("word")) ext = ".doc";
                        else if (ct != null && ct.contains("sheet")) ext = ".xls";
                        else if (ct != null && ct.contains("presentation")) ext = ".ppt";
                        File out = File.createTempFile("doc_", ext, getCacheDir());
                        try (InputStream in = conn.getInputStream();
                             FileOutputStream fos = new FileOutputStream(out)) {
                            byte[] buf = new byte[8192];
                            int n;
                            while ((n = in.read(buf)) != -1) {
                                fos.write(buf, 0, n);
                            }
                        }
                        Uri uri = FileProvider.getUriForFile(
                            MainActivity.this,
                            getApplicationContext().getPackageName() + ".fileprovider",
                            out);
                        Intent intent = new Intent(Intent.ACTION_VIEW);
                        intent.setDataAndType(uri, conn.getContentType() != null ? conn.getContentType() : "application/pdf");
                        intent.addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION);
                        intent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
                        runOnUiThread(() -> startActivity(intent));
                    } catch (Exception e) {
                        android.util.Log.e("MLDocument", "openDocument failed", e);
                    }
                }).start();
            }
        }, "MLSafeInsets");

        // The remembered-server list is shared between every origin the WebView
        // visits (the bundled gate at https://localhost and each self-hosted
        // server origin). localStorage is origin-scoped, so a list saved by the
        // gate would be invisible to the login page served from the user's
        // server — hence native SharedPreferences, plus a small JSON holder.
        final SharedPreferences prefs = getSharedPreferences("ml_servers", MODE_PRIVATE);
        webView.addJavascriptInterface(new Object() {
            @JavascriptInterface
            public String getServerState() {
                return prefs.getString("state", "");
            }

            @JavascriptInterface
            public void setServerState(final String json) {
                runOnUiThread(() -> prefs.edit().putString("state", json).apply());
            }
        }, "MLServerStore");

        // The SPA renders its login form asynchronously (auth probe first), so
        // the platform autofill framework may have already scanned the page
        // before any username/password input existed. The web side calls this
        // once the form is mounted so Bitwarden et al. get a fresh rescan.
        webView.addJavascriptInterface(new Object() {
            @JavascriptInterface
            public void rescanAutofill() {
                runOnUiThread(() -> {
                    if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                        AutofillManager afm = getSystemService(AutofillManager.class);
                        if (afm != null) {
                            afm.notifyViewEntered(webView);
                        }
                    }
                });
            }
        }, "MLAutofill");

        ViewCompat.setOnApplyWindowInsetsListener(getWindow().getDecorView(), (v, insets) -> {
            // getSystemWindowInset* covers the status bar, the navigation bar
            // and the gesture-navigation area; caption bars are covered by the
            // navigation-bar inset on the devices that use it. The display
            // cutout (punch-hole cameras) is merged in so controls stay clear
            // of it in every orientation.
            int top = insets.getSystemWindowInsetTop();
            int bottom = insets.getSystemWindowInsetBottom();
            int left = insets.getSystemWindowInsetLeft();
            int right = insets.getSystemWindowInsetRight();
            DisplayCutoutCompat cutout = insets.getDisplayCutout();
            if (cutout != null) {
                top = Math.max(top, cutout.getSafeInsetTop());
                bottom = Math.max(bottom, cutout.getSafeInsetBottom());
                left = Math.max(left, cutout.getSafeInsetLeft());
                right = Math.max(right, cutout.getSafeInsetRight());
            }
            if (safeTop != top || safeBottom != bottom || safeLeft != left || safeRight != right) {
                safeTop = top;
                safeBottom = bottom;
                safeLeft = left;
                safeRight = right;
                // Push the change into the page (the values are read lazily
                // through this interface): re-sync the --ml-safe-* variables so
                // chrome moves the moment bars hide for fullscreen or reappear
                // on exit.
                webView.evaluateJavascript("window.dispatchEvent(new Event('ml-insets'))", null);
            }
            // Keep the WebView full-bleed: the SPA shifts its own chrome via the
            // --ml-safe-* CSS variables read from the interface above.
            return insets;
        });
        ViewCompat.requestApplyInsets(getWindow().getDecorView());
    }
}