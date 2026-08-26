import type { CapacitorConfig } from "@capacitor/cli";

const config: CapacitorConfig = {
  appId: "com.icegood.medialibrary",
  appName: "Media Library",
  webDir: "dist",
  // The app boots from bundled assets, asks for (or recalls) the self-hosted
  // server address and then navigates the WebView there, so every host must
  // stay inside the WebView rather than being handed to the system browser.
  server: {
    androidScheme: "https",
    allowNavigation: ["*"]
  }
};
export default config;
