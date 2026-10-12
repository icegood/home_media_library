import { useEffect, useState } from "react";

// Walking mode: the map follows the viewer's own position and tells how far
// away (and in which direction) every geotagged media was taken.

export type MyPosition = {lat:number; lng:number; accuracy:number};

// The Android app loads the self-hosted server, often over plain HTTP, where
// the WebView refuses navigator.geolocation (secure origins only). MainActivity
// therefore exposes MLLocation, which reports fixes from the platform location
// service as "ml-location" / "ml-location-error" window events.
declare global {
  interface Window {
    MLLocation?: {
      start(): void;
      stop(): void;
    };
  }
  interface WindowEventMap {
    "ml-location": CustomEvent<MyPosition>;
    "ml-location-error": CustomEvent<string>;
  }
}

const EARTH_RADIUS_M = 6371008.8;

function radians(degrees:number) {
  return degrees * Math.PI / 180;
}

// Great-circle distance in meters (haversine).
export function distanceMeters(from:[number,number], to:[number,number]) {
  const dLat = radians(to[0] - from[0]);
  const dLng = radians(to[1] - from[1]);
  const a = Math.sin(dLat / 2) ** 2 + Math.cos(radians(from[0])) * Math.cos(radians(to[0])) * Math.sin(dLng / 2) ** 2;
  return 2 * EARTH_RADIUS_M * Math.asin(Math.min(1, Math.sqrt(a)));
}

// Initial bearing from → to in degrees clockwise from north (0..360).
export function bearingDegrees(from:[number,number], to:[number,number]) {
  const lat1 = radians(from[0]);
  const lat2 = radians(to[0]);
  const dLng = radians(to[1] - from[1]);
  const y = Math.sin(dLng) * Math.cos(lat2);
  const x = Math.cos(lat1) * Math.sin(lat2) - Math.sin(lat1) * Math.cos(lat2) * Math.cos(dLng);
  return (Math.atan2(y, x) * 180 / Math.PI + 360) % 360;
}

const COMPASS = ["N", "NE", "E", "SE", "S", "SW", "W", "NW"];

export function compassPoint(bearing:number) {
  return COMPASS[Math.round(((bearing % 360) + 360) % 360 / 45) % 8];
}

// Walking-scale formatting: whole meters up to 1 km, then km with one decimal
// below 10 km and whole km beyond.
export function formatDistance(meters:number) {
  if (meters < 1000) return `${Math.round(meters)} m`;
  if (meters < 10000) return `${(meters / 1000).toFixed(1)} km`;
  return `${Math.round(meters / 1000)} km`;
}

// Walking radius in meters, typed in by the user: the "Near you" list only
// shows media taken inside this circle, which is also drawn on the map.
export const DEFAULT_WALK_RADIUS = 1000;
const MAX_WALK_RADIUS = 20000000; // half the Earth's circumference
const WALK_RADIUS_KEY = "ml_walk_radius";

// Whole meters from 1 up to MAX_WALK_RADIUS; anything else is rejected (null).
export function parseWalkRadius(value:string) {
  const meters = Math.round(Number(value.trim()));
  return value.trim() !== "" && Number.isFinite(meters) && meters >= 1 ? Math.min(meters, MAX_WALK_RADIUS) : null;
}

// The radius is a per-device convenience; storage may be unavailable.
export function loadWalkRadius() {
  try {
    return parseWalkRadius(localStorage.getItem(WALK_RADIUS_KEY) ?? "") ?? DEFAULT_WALK_RADIUS;
  } catch {
    return DEFAULT_WALK_RADIUS;
  }
}

export function saveWalkRadius(meters:number) {
  try {
    localStorage.setItem(WALK_RADIUS_KEY, String(meters));
  } catch {
    // Not persisted; the in-memory choice still applies.
  }
}

// Watches the device position while enabled. The native bridge wins when
// present; otherwise the browser Geolocation API is used, which only works on
// secure (HTTPS or localhost) pages.
export function useMyPosition(enabled:boolean) {
  const [position, setPosition] = useState<MyPosition|null>(null);
  const [error, setError] = useState("");
  useEffect(() => {
    if (!enabled) {
      setPosition(null);
      setError("");
      return;
    }
    const native = window.MLLocation;
    if (native) {
      const onFix = (event:CustomEvent<MyPosition>) => { setPosition(event.detail); setError(""); };
      const onError = (event:CustomEvent<string>) => setError(event.detail || "Location is unavailable.");
      window.addEventListener("ml-location", onFix);
      window.addEventListener("ml-location-error", onError);
      native.start();
      return () => {
        native.stop();
        window.removeEventListener("ml-location", onFix);
        window.removeEventListener("ml-location-error", onError);
      };
    }
    if (!("geolocation" in navigator)) {
      setError("This device does not report its location.");
      return;
    }
    if (!window.isSecureContext) {
      setError("The browser only shares your location over HTTPS. Open the server over HTTPS or use the Android app.");
      return;
    }
    const watch = navigator.geolocation.watchPosition(
      fix => {
        setPosition({lat:fix.coords.latitude, lng:fix.coords.longitude, accuracy:fix.coords.accuracy});
        setError("");
      },
      cause => setError(cause.code === cause.PERMISSION_DENIED ? "Location permission was denied." : cause.message || "Location is unavailable."),
      {enableHighAccuracy:true, maximumAge:5000, timeout:30000}
    );
    return () => navigator.geolocation.clearWatch(watch);
  }, [enabled]);
  return {position, error};
}
