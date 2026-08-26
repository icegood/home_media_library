import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, expect, test } from "vitest";
import { applyLanguageSetting, currentSetting, installDomTranslation, LANGUAGES, onLanguageChange, resolvedLanguage, useLanguage } from "./i18n";

function setBrowserLocale(locale:string|undefined) {
  Object.defineProperty(window.navigator, "language", {value:locale, configurable:true});
}

function translateSnippet(html:string) {
  document.body.innerHTML = html;
  installDomTranslation();
  return document.body;
}

beforeEach(() => {
  setBrowserLocale("en-US");
  applyLanguageSetting("auto");
});

afterEach(() => {
  cleanup();
  document.body.innerHTML = "";
  setBrowserLocale("en-US");
  applyLanguageSetting("en");
});

test("settings resolve to a supported language and normalize unknown values", () => {
  expect(LANGUAGES.map(entry => entry.id)).toEqual(["en", "ua", "de", "nl", "pl", "che", "slo", "hu", "fi", "sv", "es", "it", "sl", "no", "pt"]);
  expect(applyLanguageSetting("de")).toBe(true);
  expect(currentSetting()).toBe("de");
  expect(resolvedLanguage()).toBe("de");
  expect(applyLanguageSetting("klingon")).toBe(true);
  expect(currentSetting()).toBe("auto");
  // Staying on the same resolved language reports no change.
  expect(applyLanguageSetting("klingon")).toBe(false);
  // Switching from "auto"(en) to explicit "en" keeps the resolution stable.
  expect(applyLanguageSetting("en")).toBe(false);
});

test("auto follows the browser locale across every supported prefix", () => {
  const cases:[string, string][] = [
    ["uk-UA", "ua"], ["de-AT", "de"], ["nl-BE", "nl"], ["pl-PL", "pl"],
    ["cs-CZ", "che"], ["sk-SK", "slo"], ["hu-HU", "hu"], ["fi-FI", "fi"],
    ["sv-SE", "sv"], ["es-MX", "es"], ["it-IT", "it"], ["sl-SI", "sl"],
    ["nb-NO", "no"], ["nn-NO", "no"], ["pt-BR", "pt"], ["ru-RU", "en"],
    ["fr-FR", "en"]
  ];
  for (const [locale, expected] of cases) {
    setBrowserLocale(locale);
    applyLanguageSetting("auto");
    expect(resolvedLanguage()).toBe(expected);
  }
});

test("change listeners fire on switches and unsubscribe cleanly", () => {
  const seen:string[] = [];
  const unsubscribe = onLanguageChange(() => seen.push(resolvedLanguage()));
  applyLanguageSetting("de");
  applyLanguageSetting("de");
  unsubscribe();
  applyLanguageSetting("es");
  expect(seen).toEqual(["de"]);
});

test("useLanguage hook re-renders components when the language changes", () => {
  const hook = renderHook(() => useLanguage());
  expect(hook.result.current).toBe("en");
  act(() => { applyLanguageSetting("hu"); });
  expect(hook.result.current).toBe("hu");
});

test("every supported language translates chrome strings and composites", () => {
  const expectations:Record<string, {brand:string; items:(html:string)=>string; stats:string; openLabel:string; menuLabel:string}> = {
    ua: {brand:"Медіатека", items:() => "12 елементів", stats:"Фото: 2 · Відео: 1", openLabel:"Відкрити Family", menuLabel:"Меню бібліотеки Family"},
    de: {brand:"Mediathek", items:() => "12 Elemente", stats:"Fotos: 2 · Videos: 1", openLabel:"Öffnen Family", menuLabel:"Bibliotheksmenü Family"},
    nl: {brand:"Mediatheek", items:() => "12 items", stats:"Foto's: 2 · Video's: 1", openLabel:"Openen Family", menuLabel:"Bibliotheekmenu Family"},
    pl: {brand:"Medioteka", items:() => "12 elementów", stats:"Zdjęcia: 2 · Filmy: 1", openLabel:"Otwórz Family", menuLabel:"Menu biblioteki Family"},
    che:{brand:"Mediální knihovna", items:() => "12 položek", stats:"Fotky: 2 · Videa: 1", openLabel:"Otevřít Family", menuLabel:"Menu knihovny Family"},
    slo:{brand:"Mediálna knižnica", items:() => "12 položiek", stats:"Fotky: 2 · Videá: 1", openLabel:"Otvoriť Family", menuLabel:"Menu knižnice Family"},
    hu: {brand:"Médiakönyvtár", items:() => "12 elem", stats:"Képek: 2 · Videók: 1", openLabel:"Megnyitás Family", menuLabel:"Könyvtár menüje Family"},
    fi: {brand:"Mediakirjasto", items:() => "12 kohdetta", stats:"Kuvat: 2 · Videot: 1", openLabel:"Avaa Family", menuLabel:"Kirjaston valikko Family"},
    sv: {brand:"Mediebibliotek", items:() => "12 objekt", stats:"Bilder: 2 · Videor: 1", openLabel:"Öppna Family", menuLabel:"Biblioteksmeny Family"},
    es: {brand:"Mediateca", items:() => "12 elementos", stats:"Fotos: 2 · Vídeos: 1", openLabel:"Abrir Family", menuLabel:"Menú de biblioteca Family"},
    it: {brand:"Mediateca", items:() => "12 elementi", stats:"Foto: 2 · Video: 1", openLabel:"Apri Family", menuLabel:"Menu libreria Family"},
    sl: {brand:"Medijska knjižnica", items:() => "12 predmetov", stats:"Fotografije: 2 · Videoposnetki: 1", openLabel:"Odpri Family", menuLabel:"Meni knjižnice Family"},
    no: {brand:"Mediebibliotek", items:() => "12 elementer", stats:"Bilder: 2 · Videoer: 1", openLabel:"Åpne Family", menuLabel:"Bibliotekmeny Family"},
    pt: {brand:"Mediateca", items:() => "12 elementos", stats:"Fotos: 2 · Vídeos: 1", openLabel:"Abrir Family", menuLabel:"Menu da biblioteca Family"}
  };
  for (const [id, expected] of Object.entries(expectations)) {
    expect(applyLanguageSetting(id)).toBe(true);
    document.body.innerHTML = `
      <a class="brand">Media Library</a>
      <span id="count">12 items</span>
      <span id="stats">Images: 2 · Videos: 1</span>
      <span id="view">Trip from this favorite view</span>
      <span id="loading">Loading 3 of 10…</span>
      <a id="thumb" aria-label="Open Family">thumb</a>
      <button id="menu" aria-label="Library menu Family">dots</button>
      <button id="rm" aria-label="Remove Trip">x</button>
      <button id="del" aria-label="Delete Trip">x</button>
      <button id="sel" aria-label="Select Trip">x</button>
      <button id="mgr" aria-label="Manage favorite views for Trip">x</button>
      <button id="vmenu" aria-label="Favorite view menu Best">x</button>
      <button id="fmenu" aria-label="Folder menu Photos">x</button>
      <button id="sortn" aria-label="Newest first: Photos">x</button>
      <button id="sorto" aria-label="Oldest first: Photos">x</button>
      <input placeholder="Search by street or place…" title="Place search query"/>
    `;
    // Grab nodes before translating: labels change under the observer.
    const thumb = document.getElementById("thumb")!;
    const menu = document.getElementById("menu")!;
    const rm = document.getElementById("rm")!;
    const del = document.getElementById("del")!;
    const sel = document.getElementById("sel")!;
    const mgr = document.getElementById("mgr")!;
    const vmenu = document.getElementById("vmenu")!;
    const fmenu = document.getElementById("fmenu")!;
    const sortn = document.getElementById("sortn")!;
    const sorto = document.getElementById("sorto")!;
    const input = document.querySelector("input")!;
    installDomTranslation();
    expect(document.querySelector(".brand")?.textContent).toBe(expected.brand);
    expect(document.getElementById("count")?.textContent).toBe(expected.items(""));
    expect(document.getElementById("stats")?.textContent).toBe(expected.stats);
    expect(document.getElementById("view")?.textContent).not.toContain("favorite view");
    expect(document.getElementById("loading")?.textContent).not.toBe("Loading 3 of 10…");
    expect(thumb.getAttribute("aria-label")).toBe(expected.openLabel);
    expect(menu.getAttribute("aria-label")).toBe(expected.menuLabel);
    expect(input.getAttribute("placeholder")).not.toContain("street");
    // Verb and menu composites exercise every grammar hook per language.
    expect(rm.getAttribute("aria-label")).not.toBe("Remove Trip");
    expect(del.getAttribute("aria-label")).not.toBe("Delete Trip");
    expect(sel.getAttribute("aria-label")).not.toBe("Select Trip");
    expect(mgr.getAttribute("aria-label")).not.toBe("Manage favorite views for Trip");
    expect(vmenu.getAttribute("aria-label")).not.toBe("Favorite view menu Best");
    expect(fmenu.getAttribute("aria-label")).not.toBe("Folder menu Photos");
    expect(sortn.getAttribute("aria-label")).toContain(": Photos");
    expect(sorto.getAttribute("aria-label")).toContain(": Photos");
    // Disconnect the live observer so the next iteration starts clean.
    applyLanguageSetting("en");
    installDomTranslation();
  }
});

test("slavic plural grammars pick the correct form per count", () => {
  const cases:[string, string, string][] = [
    ["ua", "1 item", "1 елемент"], ["ua", "4 items", "4 елементи"], ["ua", "11 items", "11 елементів"],
    ["pl", "1 item", "1 element"], ["pl", "3 items", "3 elementy"], ["pl", "13 items", "13 elementów"],
    ["che", "1 item", "1 položka"], ["che", "2 items", "2 položky"], ["che", "6 items", "6 položek"],
    ["slo", "1 item", "1 položka"], ["slo", "4 items", "4 položky"], ["slo", "8 items", "8 položiek"],
    ["sl", "1 item", "1 predmet"], ["sl", "2 items", "2 predmeta"], ["sl", "3 items", "3 predmeti"], ["sl", "9 items", "9 predmetov"],
    ["hu", "1 item", "1 elem"], ["hu", "99 items", "99 elem"]
  ];
  for (const [lang, source, expected] of cases) {
    applyLanguageSetting(lang as never);
    document.body.innerHTML = `<span>${source}</span>`;
    installDomTranslation();
    expect(document.body.textContent).toBe(expected);
  }
});

test("attribute composites cover verbs menus and sorting labels", () => {
  applyLanguageSetting("che");
  const body = translateSnippet(`
    <button aria-label="Remove one.jpg">x</button>
    <button aria-label="Delete one.jpg">x</button>
    <button aria-label="Select one.jpg">x</button>
    <button aria-label="Manage favorite views for Trip">x</button>
    <button aria-label="Favorite view menu Best">x</button>
    <button aria-label="Folder menu Photos">x</button>
    <button aria-label="Newest first: Photos">x</button>
    <button aria-label="Oldest first: Photos">x</button>
  `);
  const labels = [...body.querySelectorAll("button")].map(button => button.getAttribute("aria-label"));
  expect(labels).toEqual([
    "Odebrat one.jpg", "Smazat one.jpg", "Vybrat one.jpg",
    "Spravovat oblíbená zobrazení pro Trip", "Menu zobrazení Best",
    "Menu složky Photos", "Nejnovější první: Photos", "Nejstarší první: Photos"
  ]);
});

test("english mode leaves the DOM untouched and disconnects the observer", () => {
  applyLanguageSetting("en");
  const before = "<span>Media Library</span>";
  document.body.innerHTML = before;
  installDomTranslation();
  expect(document.body.innerHTML).toBe(before);
});

test("untranslated strings pass through unchanged", () => {
  applyLanguageSetting("de");
  document.body.innerHTML = "<span>Totally custom value</span><code>Media Library</code>";
  installDomTranslation();
  expect(document.body.textContent).toContain("Totally custom value");
  expect(document.body.querySelector("code")?.textContent).toBe("Media Library");
});

test("translation helpers pass through unmatched composites safely", () => {
  applyLanguageSetting("de");
  document.body.innerHTML = `
    <span id="a">12 itemss</span>
    <span id="b">Images: x</span>
    <span id="c">Loading of…</span>
    <button id="d" aria-label="Frobnicate Trip">x</button>
    <input aria-label=""/>
  `;
  installDomTranslation();
  expect(document.getElementById("a")?.textContent).toBe("12 itemss");
  expect(document.getElementById("b")?.textContent).toBe("Images: x");
  expect(document.getElementById("c")?.textContent).toBe("Loading of…");
  expect(document.getElementById("d")?.getAttribute("aria-label")).toBe("Frobnicate Trip");
  applyLanguageSetting("en");
  installDomTranslation();
});
