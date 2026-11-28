import { fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { expect, test } from "vitest";
import { applyMarkdown, linkLabel, NotesEditor, NotesMarkdown } from "./notes-markdown";

function html(text:string) {
  return render(<NotesMarkdown text={text}/>).container.firstElementChild as HTMLElement;
}

test("bare links become clickable, open in a new tab and drop a closing bracket around them", () => {
  const full = "https://www.google.com/maps/place/50%C2%B026'22.6%22N+30%C2%B024'47.1%22E/@50.4396401,30.4131581,184a/data=!3m7?entry=tts&skid=d47db2d6";
  const root = html(`дерево схоже на\nhttps://maps.app.goo.gl/cypzAhTDBBZWDR4Y6 (${full})\nмагазин.`);
  const links = Array.from(root.querySelectorAll("a"));
  expect(links.map(a => a.getAttribute("href"))).toEqual(["https://maps.app.goo.gl/cypzAhTDBBZWDR4Y6", full]);
  expect(links[1]).toHaveAttribute("target", "_blank");
  expect(links[1]).toHaveAttribute("rel", "noopener noreferrer");
  // The bracket stays text; the long address shows a shortened, decoded label.
  expect(root.textContent).toContain(")");
  expect(links[1].textContent).toBe(linkLabel(full));
  expect(links[1].textContent!.startsWith("google.com/maps/place/50°26'22.6\"N")).toBe(true);
  expect(links[1].textContent!.endsWith("…")).toBe(true);
  // Lines stay lines.
  expect(root.querySelectorAll("br")).toHaveLength(2);
});

test("markdown links, emphasis, code, lists and headings render as elements", () => {
  const root = html("# Trip\nSee [the map](https://example.com/m) and **bold** *soft* `x=1`\n\n- one\n- two https://a.b/c.");
  expect(root.querySelector("strong.notes-heading")?.textContent).toBe("Trip");
  const link = root.querySelector("a")!;
  expect(link.textContent).toBe("the map");
  expect(link).toHaveAttribute("href", "https://example.com/m");
  expect(root.querySelector("p strong")?.textContent).toBe("bold");
  expect(root.querySelector("em")?.textContent).toBe("soft");
  expect(root.querySelector("code")?.textContent).toBe("x=1");
  expect(Array.from(root.querySelectorAll("li")).map(li => li.textContent)).toEqual(["one", "two a.b/c."]);
  expect(root.querySelectorAll("li a")[0]).toHaveAttribute("href", "https://a.b/c");
});

test("unsafe link targets and raw HTML stay plain text", () => {
  const root = html("[click](javascript:alert(1)) <img src=x onerror=alert(1)> data:text/html,hi");
  expect(root.querySelector("a")).toBeNull();
  expect(root.querySelector("img")).toBeNull();
  expect(root.textContent).toContain("<img src=x onerror=alert(1)>");
  expect(root).toHaveAttribute("translate", "no");
});

test("toolbar actions write Markdown around the selection or insert a selected placeholder", () => {
  expect(applyMarkdown("make this bold", 5, 9, "bold")).toEqual({value:"make **this** bold", start:7, end:11});
  expect(applyMarkdown("ab", 1, 1, "italic")).toEqual({value:"a*italic text*b", start:2, end:13});
  expect(applyMarkdown("x", 1, 1, "code").value).toBe("x`code`");
  const link = applyMarkdown("see https://a.b/c now", 4, 17, "link");
  expect(link.value).toBe("see [link](https://a.b/c) now");
  expect(link.value.slice(link.start, link.end)).toBe("link");
  const named = applyMarkdown("the park", 4, 8, "link");
  expect(named.value).toBe("the [park](https://)");
  expect(named.value.slice(named.start, named.end)).toBe("https://");
  expect(applyMarkdown("one\ntwo\nthree", 0, 7, "list").value).toBe("- one\n- two\nthree");
  expect(applyMarkdown("- one\ntwo", 0, 9, "list").value).toBe("- one\n- two");
  expect(applyMarkdown("intro\ntitle", 8, 8, "heading").value).toBe("intro\n## title");
});

test("the notes editor shows one view at a time and remembers Plain or Markdown", () => {
  localStorage.removeItem("media-library-notes-mode");
  function Harness({initial}:{initial:string}) {
    const [value, setValue] = useState(initial);
    return <NotesEditor value={value} onChange={setValue}/>;
  }
  const {unmount} = render(<Harness initial="**hi** https://a.b"/>);
  // Markdown mode opens formatted when there is a note.
  expect(screen.getByLabelText("Formatted notes").querySelector("strong")?.textContent).toBe("hi");
  expect(screen.queryByRole("textbox")).toBeNull();
  fireEvent.click(screen.getByRole("button", {name:"Edit"}));
  const box = screen.getByRole("textbox") as HTMLTextAreaElement;
  expect(screen.queryByLabelText("Formatted notes")).toBeNull();
  box.setSelectionRange(0, 0);
  fireEvent.click(screen.getByRole("button", {name:"Heading"}));
  expect(box.value).toBe("## **hi** https://a.b");
  fireEvent.click(screen.getByRole("button", {name:"View"}));
  expect(screen.getByLabelText("Formatted notes").querySelector(".notes-heading")).not.toBeNull();
  // Plain: just the text box, no toolbar, remembered for next time.
  fireEvent.click(screen.getByRole("button", {name:"Plain"}));
  expect(screen.getByRole("textbox")).toHaveValue("## **hi** https://a.b");
  expect(screen.queryByRole("toolbar")).toBeNull();
  expect(screen.queryByLabelText("Formatted notes")).toBeNull();
  unmount();
  render(<Harness initial="note"/>);
  expect(screen.getByRole("button", {name:"Plain"})).toHaveAttribute("aria-pressed", "true");
  expect(screen.getByRole("textbox")).toHaveValue("note");
  localStorage.removeItem("media-library-notes-mode");
});
