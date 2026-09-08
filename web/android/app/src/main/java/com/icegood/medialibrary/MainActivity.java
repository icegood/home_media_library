package com.icegood.medialibrary;

import android.os.Bundle;
import android.webkit.WebSettings;
import android.webkit.WebView;

import com.getcapacitor.BridgeActivity;

public class MainActivity extends BridgeActivity {
    @Override
    public void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        // The app boots from the Capacitor origin (https://localhost) and then
        // fetches from the user's self-hosted server, which may be plain HTTP
        // (e.g. LAN-only installs). Without this, those http fetches are treated
        // as mixed content and blocked, so the server-address form fails while
        // the same URL works fine in the system browser.
        if (getBridge() != null && getBridge().getWebView() != null) {
            getBridge().getWebView().getSettings().setMixedContentMode(
                    WebSettings.MIXED_CONTENT_ALWAYS_ALLOW);
            WebView.setWebContentsDebuggingEnabled(true);
        }
    }
}
