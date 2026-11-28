import { readFileSync } from "node:fs";
import { join } from "node:path";
import ts from "typescript";
import { expect, test } from "vitest";

// The UI is translated by rewriting DOM text nodes after React renders them,
// one node at a time (exact entries and "%s" templates). JSX text written next
// to a value — <p>Imported {n} libraries</p> — renders as separate nodes
// ("Imported ", "5", " libraries"), so no entry ever matches and the sentence
// stays English. Such labels must be built as one string instead:
// <p>{`Imported ${n} libraries`}</p> plus a "%s" template row.
const SOURCES = ["App.tsx"];

// Fragments with no word to translate (units, symbols) may sit next to a value.
const UNTRANSLATABLE = /^(px|%|×|x)$/;

function containsJsx(node:ts.Node):boolean {
  if (ts.isJsxElement(node) || ts.isJsxSelfClosingElement(node) || ts.isJsxFragment(node)) return true;
  return ts.forEachChild(node, containsJsx) ?? false;
}

// A {value} that renders text: not empty, not a comment, not markup.
function rendersText(child:ts.JsxChild) {
  return ts.isJsxExpression(child) && child.expression != null && !containsJsx(child.expression);
}

function splitLabels(file:string) {
  const text = readFileSync(join(process.cwd(), "src", file), "utf8");
  const source = ts.createSourceFile(file, text, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
  const found:string[] = [];
  function visit(node:ts.Node) {
    if (ts.isJsxElement(node) || ts.isJsxFragment(node)) {
      const children = node.children.filter(child => !(ts.isJsxText(child) && child.containsOnlyTriviaWhiteSpaces));
      children.forEach((child, index) => {
        if (!ts.isJsxText(child)) return;
        const words = child.text.replace(/\s+/g, " ").trim();
        if (!/[A-Za-z]{2,}/.test(words) || UNTRANSLATABLE.test(words)) return;
        const before = children[index - 1];
        const after = children[index + 1];
        if ((before && rendersText(before)) || (after && rendersText(after))) {
          const line = source.getLineAndCharacterOfPosition(child.getStart()).line + 1;
          found.push(`${file}:${line} "${words}"`);
        }
      });
    }
    ts.forEachChild(node, visit);
  }
  visit(source);
  return found;
}

test("no visible label mixes JSX text with a value, so every sentence reaches the translator whole", () => {
  expect(SOURCES.flatMap(splitLabels)).toEqual([]);
});
