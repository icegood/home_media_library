import type { ReactNode } from "react";

// A small, safe Markdown subset for media notes. It builds React elements
// (never HTML strings), so a note cannot inject markup or scripts.
//   blocks: "# " / "## " / "### " headings, "- " or "* " bullet lists,
//           blank line = new paragraph, single newline = line break
//   inline: [text](url) links, bare http(s):// links, **bold**, *italic*,
//           `code`
// Only http(s) and mailto links become clickable; anything else stays text.

const SAFE_URL = /^(https?:\/\/|mailto:)/i;
const LINK_LABEL_MAX = 60;

// code | [text](url) | bare url | **bold** | *italic*
const INLINE = /`([^`\n]+)`|\[([^\]\n]+)\]\(([^)\s]+)\)|(https?:\/\/[^\s<>"]+)|\*\*([^*\n]+)\*\*|\*([^*\s][^*\n]*)\*/g;

// A bare URL ends before trailing punctuation, and before a closing bracket
// it does not open itself: "see (https://x.y/a)" links https://x.y/a.
function trimUrl(raw:string):string {
  let url = raw.replace(/[.,;:!?'"]+$/, "");
  while (url.endsWith(")") && (url.match(/\(/g)?.length ?? 0) < (url.match(/\)/g)?.length ?? 0)) url = url.slice(0, -1);
  return url;
}

// Long links (full Google Maps addresses) show a readable, shortened label.
export function linkLabel(url:string):string {
  let label = url.replace(/^https?:\/\/(www\.)?/i, "");
  try { label = decodeURIComponent(label); } catch { /* keep it encoded */ }
  return label.length > LINK_LABEL_MAX ? `${label.slice(0, LINK_LABEL_MAX - 1)}…` : label;
}

function link(url:string, label:ReactNode, key:string):ReactNode {
  if (!SAFE_URL.test(url)) return label;
  return <a key={key} href={url} target="_blank" rel="noopener noreferrer" title={url}
    onClick={event => event.stopPropagation()}>{label}</a>;
}

function inline(text:string, keyPrefix:string):ReactNode[] {
  const out:ReactNode[] = [];
  const pattern = new RegExp(INLINE.source, "g");
  let last = 0;
  let index = 0;
  for (let match = pattern.exec(text); match; match = pattern.exec(text)) {
    const start = match.index;
    const key = `${keyPrefix}-${index++}`;
    const [whole, code, linkText, linkUrl, bare, bold, italic] = match;
    let consumed = whole.length;
    let node:ReactNode;
    if (code != null) node = <code key={key}>{code}</code>;
    else if (linkText != null) node = link(linkUrl, inline(linkText, key), key);
    else if (bare != null) {
      const url = trimUrl(bare);
      consumed = url.length;
      node = link(url, linkLabel(url), key);
    } else if (bold != null) node = <strong key={key}>{inline(bold, key)}</strong>;
    else node = <em key={key}>{inline(italic, key)}</em>;
    if (start > last) out.push(text.slice(last, start));
    out.push(node);
    last = start + consumed;
    // A trimmed URL gives its tail back to the text; resume scanning there.
    pattern.lastIndex = last;
  }
  if (last < text.length) out.push(text.slice(last));
  return out;
}

export function NotesMarkdown({text, className = "notes-text"}:{text:string; className?:string}) {
  const blocks:ReactNode[] = [];
  let lines:string[] = [];
  let items:string[] = [];
  const flushParagraph = () => {
    if (lines.length === 0) return;
    const key = `p${blocks.length}`;
    blocks.push(<p key={key}>{lines.flatMap((line, i) => i === 0 ? inline(line, `${key}-${i}`) : [<br key={`${key}-br${i}`}/>, ...inline(line, `${key}-${i}`)])}</p>);
    lines = [];
  };
  const flushList = () => {
    if (items.length === 0) return;
    const key = `ul${blocks.length}`;
    blocks.push(<ul key={key}>{items.map((item, i) => <li key={i}>{inline(item, `${key}-${i}`)}</li>)}</ul>);
    items = [];
  };
  for (const raw of text.replace(/\r\n?/g, "\n").split("\n")) {
    const line = raw.trimEnd();
    const bullet = line.match(/^\s*[-*]\s+(.*)$/);
    const heading = line.match(/^(#{1,3})\s+(.*)$/);
    if (bullet) { flushParagraph(); items.push(bullet[1]); continue; }
    flushList();
    if (heading) {
      flushParagraph();
      const key = `h${blocks.length}`;
      blocks.push(<strong key={key} className={`notes-heading notes-h${heading[1].length}`}>{inline(heading[2], key)}</strong>);
      continue;
    }
    if (line.trim() === "") { flushParagraph(); continue; }
    lines.push(line);
  }
  flushParagraph();
  flushList();
  // translate="no": a note is the user's own text, never UI to translate.
  return <div className={`${className} notes-markdown`} translate="no">{blocks}</div>;
}
