import { useEffect, useRef, useState, type ReactNode } from "react";

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

export type MarkdownAction = "bold" | "italic" | "link" | "list" | "heading" | "code";

// applyMarkdown inserts the syntax for one toolbar action around the selection
// [start, end) of value; with nothing selected it inserts a placeholder and
// selects it, so typing replaces it. Returns the new text and selection.
export function applyMarkdown(value:string, start:number, end:number, action:MarkdownAction):{value:string; start:number; end:number} {
  const selected = value.slice(start, end);
  const wrap = (before:string, after:string, placeholder:string) => {
    const inner = selected || placeholder;
    const next = value.slice(0, start) + before + inner + after + value.slice(end);
    return {value: next, start: start + before.length, end: start + before.length + inner.length};
  };
  const prefixLines = (prefix:string) => {
    const lineStart = value.lastIndexOf("\n", start - 1) + 1;
    const lineEnd = end > start && value[end - 1] === "\n" ? end - 1 : (value.indexOf("\n", end) === -1 ? value.length : value.indexOf("\n", end));
    const block = value.slice(lineStart, lineEnd);
    const prefixed = block.split("\n").map(line => line.startsWith(prefix) ? line : prefix + line).join("\n");
    return {value: value.slice(0, lineStart) + prefixed + value.slice(lineEnd), start: lineStart, end: lineStart + prefixed.length};
  };
  switch (action) {
    case "bold": return wrap("**", "**", "bold text");
    case "italic": return wrap("*", "*", "italic text");
    case "code": return wrap("`", "`", "code");
    case "list": return prefixLines("- ");
    case "heading": return prefixLines("## ");
    case "link": {
      if (/^https?:\/\/\S+$/.test(selected)) {
        const next = value.slice(0, start) + `[link](${selected})` + value.slice(end);
        return {value: next, start: start + 1, end: start + 5};
      }
      const label = selected || "text";
      const next = value.slice(0, start) + `[${label}](https://)` + value.slice(end);
      const urlStart = start + label.length + 3;
      return {value: next, start: urlStart, end: urlStart + "https://".length};
    }
  }
}

const TOOLBAR:{action:MarkdownAction; label:string; text:string}[] = [
  {action:"bold", label:"Bold", text:"B"},
  {action:"italic", label:"Italic", text:"I"},
  {action:"link", label:"Link", text:"🔗"},
  {action:"list", label:"Bulleted list", text:"•"},
  {action:"heading", label:"Heading", text:"H"},
  {action:"code", label:"Code", text:"</>"},
];

// NotesEditor shows one view at a time: the formatted note, or (Edit) the
// text box with a toolbar that writes the Markdown syntax.
export function NotesEditor({value, onChange, placeholder}:{value:string; onChange:(value:string)=>void; placeholder?:string}) {
  const [editing, setEditing] = useState(() => value.trim() === "");
  const textareaRef = useRef<HTMLTextAreaElement|null>(null);
  const pendingSelection = useRef<{start:number; end:number}|null>(null);
  useEffect(() => {
    const selection = pendingSelection.current;
    const textarea = textareaRef.current;
    if (!selection || !textarea) return;
    pendingSelection.current = null;
    textarea.focus();
    textarea.setSelectionRange(selection.start, selection.end);
  });
  function apply(action:MarkdownAction) {
    const textarea = textareaRef.current;
    const start = textarea?.selectionStart ?? value.length;
    const end = textarea?.selectionEnd ?? value.length;
    const result = applyMarkdown(value, start, end, action);
    pendingSelection.current = {start: result.start, end: result.end};
    onChange(result.value);
  }
  return <div className="notes-editor">
    <div className="notes-editor-bar">
      {editing ? <div className="notes-toolbar" role="toolbar" aria-label="Formatting">
        {TOOLBAR.map(tool => <button key={tool.action} type="button" aria-label={tool.label} title={tool.label}
          onMouseDown={event => event.preventDefault()} onClick={() => apply(tool.action)}>{tool.text}</button>)}
      </div> : <span/>}
      <button type="button" className="secondary notes-edit-toggle" onClick={() => setEditing(current => !current)}>{editing ? "View" : "Edit"}</button>
    </div>
    {editing
      ? <textarea ref={textareaRef} className="notes-editor-input" aria-label="Notes" value={value} onChange={event => onChange(event.target.value)} placeholder={placeholder}/>
      : <div className="notes-editor-view" aria-label="Formatted notes" onDoubleClick={() => setEditing(true)}>
          {value.trim() ? <NotesMarkdown text={value}/> : <p className="muted">No notes yet.</p>}
        </div>}
  </div>;
}
