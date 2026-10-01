#!/usr/bin/env python3
"""Build the website's documentation (site/docs/) from the Markdown in the
repository, with the standard library only.

Every page gets the site's header, a sidebar with the documentation split
into sections, and "On this page" from its headings. The user guide is
split into chapters, one page each. Links between documents become links
between pages; anything else points at the file on GitHub.

    python3 scripts/site_docs.py [OUT]     (default: site/docs)
"""

import html
import os
import re
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
REPO = "https://github.com/turbine-dev/pimpo"

# The user guide's chapters: page, title, summary, and the ## sections in it.
GUIDE = [
    ("guide-start", "Getting started", "Install Pimpo, find your way around and ask for the first task.",
     ["Getting around", "First steps", "Pimpo, the mascot", "Language", "Chat"]),
    ("guide-routines", "Routines", "How a task becomes a routine, what starts it, and how to change it.",
     ["How a task becomes a routine", "Routines that react to something new", "Routines that remember, and routines built from others",
      "Routines in a repository", "Changing a routine without code", "Run history", "Routines that ask you", "Reminders", "Long jobs", "Suggestions"]),
    ("guide-dashboards", "Dashboards", "Tabs of widgets: routines as widgets, ready-made ones, charts and sharing.",
     ["Dashboards"]),
    ("guide-companies", "Companies of agents", "Companies of any kind whose members are agents, with you as the CEO.",
     ["Companies of agents"]),
    ("guide-safety", "Approvals and receipts", "What waits for you, and how to see and undo what Pimpo did.",
     ["Approvals and rules", "Receipts and undo"]),
    ("guide-memory", "Memory and people", "What Pimpo remembers, and sharing it with the people of the house.",
     ["Memory", "People"]),
    ("guide-devices", "Phone, voice and devices", "The phone app, other computers and servers, and talking to Pimpo.",
     ["On the phone", "A Pimpo on another computer or server", "Audio"]),
    ("guide-channels", "Channels and webhooks", "Telegram, WhatsApp, Slack, Discord, Signal, iMessage, email and webhooks.",
     ["Other chat channels", "Webhooks"]),
    ("guide-connections", "Connections", "Accounts, connectors, the gallery, skills, the browser and running code.",
     ["Google Sheets", "Spotify", "Apple Reminders, Notes and Calendar", "More connectors", "Gallery", "Skills", "Using sites without an API", "Running code"]),
    ("guide-models", "Models and costs", "Choosing models, downloading local ones, and what everything costs.",
     ["Models", "Downloading models", "Costs"]),
    ("guide-care", "Updates, backups and help", "Keeping Pimpo up to date, moving it, backups and check-ups.",
     ["Updates", "Moving and backups", "Check-up", "Help, notifications and labs"]),
]

# The sidebar: sections of (page, source file, title, summary).
SECTIONS = [
    ("Start here", [
        ("index", None, "Overview", "Where to start, and everything the documentation covers."),
        ("guide", "docs/USER_GUIDE.md", "User guide", "Everything you can do with Pimpo, screen by screen."),
    ] + [(slug, "guide", title, summary) for slug, title, summary, _ in GUIDE]),
    ("In depth", [
        ("routines", "docs/ROUTINES.md", "How routines work", "From a request to a tested routine: triggers, tests and writing one by hand."),
        ("configuration", "docs/CONFIGURATION.md", "Configuration", "The command line, settings, models, channels, the data folder, network access and backups."),
        ("self-hosting", "docs/SELF_HOSTING.md", "Running on a server", "Docker, systemd, reaching it safely, and the apps opening it."),
        ("validation", "docs/VALIDATION.md", "Validating on real accounts", "What `pimpo report` measures and what counts as done."),
    ]),
    ("Extending", [
        ("sdk", "docs/SDK.md", "Extending Pimpo", "The API, channels, bridges and everything a developer can build on."),
        ("connectors", "docs/CONNECTORS.md", "Writing a connector", "JSON connectors, MCP servers and OpenAPI imports."),
        ("formats", "docs/FORMATS.md", "Data formats", "The frozen formats of the database, routines, connectors, memory and backups."),
        ("rfc-0001", "docs/rfcs/0001-code-sandbox.md", "RFC 0001: code sandbox", "How the agent runs code in isolation."),
        ("rfc-0002", "docs/rfcs/0002-browser.md", "RFC 0002: browser", "How the agent drives a browser safely."),
        ("rfc-0003", "docs/rfcs/0003-dashboards.md", "RFC 0003: dashboards and widgets", "Dashboards, routines as widgets, and widgets on every system."),
        ("rfc-0004", "docs/rfcs/0004-companies.md", "RFC 0004: companies of agents", "Companies with roles, members, decision levels and accounts of their own."),
        ("rfc-template", "docs/rfcs/0000-template.md", "RFC template", "How to propose a larger change."),
    ]),
    ("Security", [
        ("threat-model", "docs/THREAT_MODEL.md", "Threat model", "What Pimpo guarantees, against whom, and the tests that prove it."),
        ("security", "SECURITY.md", "Reporting a vulnerability", "How to report a security problem privately."),
    ]),
    ("Project", [
        ("roadmap", "docs/ROADMAP.md", "Roadmap", "The milestones to v1.0 and beyond."),
        ("status", "docs/STATUS.md", "Status", "What is built, what is tested, and what waits."),
        ("releases", "docs/RELEASES.md", "Releases and support", "Versions, channels, signing and how a release is made."),
        ("planning", "docs/PLANNING.md", "The plan", "Why Pimpo exists and how it was planned."),
        ("changelog", "CHANGELOG.md", "Changelog", "What changed in each version."),
        ("contributing", "CONTRIBUTING.md", "Contributing", "How to help, and how changes are made."),
        ("governance", "GOVERNANCE.md", "Governance", "How decisions are made."),
        ("code-of-conduct", "CODE_OF_CONDUCT.md", "Code of Conduct", "How we treat each other."),
        ("support", "SUPPORT.md", "Getting help", "Where to ask questions and report problems."),
    ]),
]


# ---------- Markdown ----------

def slugify(text):
    """GitHub's heading anchors: lower case, punctuation dropped, spaces to dashes."""
    t = re.sub(r"<[^>]+>", "", text).strip().lower()
    t = re.sub(r"[^\w\- ]", "", t, flags=re.UNICODE)
    return t.replace(" ", "-")


RAW_BLOCK = re.compile(r"^\s*</?(p|img|div|h[1-6]|br|details|summary|table|picture|source|a)\b", re.I)


class Converter:
    def __init__(self, link):
        self.link = link  # rewrites a link target
        self.ids = {}
        self.toc = []

    def anchor(self, text):
        base = slugify(text) or "section"
        n = self.ids.get(base, 0)
        self.ids[base] = n + 1
        return base if n == 0 else f"{base}-{n}"

    def anchor_tag(self, m):
        href = self.link(html.unescape(m.group(2)))
        label = m.group(1)
        # A link written as a file name reads as the page's title here.
        page = href.split("#")[0]
        if re.fullmatch(r"(?:\x00\d+\x00|[\w./-]+\.md)", label) and page in TITLES:
            label = html.escape(TITLES[page])
        return f'<a href="{html.escape(href)}">{label}</a>'

    def inline(self, text):
        codes = []

        def keep_code(m):
            codes.append("<code>" + html.escape(m.group(2)) + "</code>")
            return f"\x00{len(codes) - 1}\x00"

        text = re.sub(r"(`+)(.+?)\1", keep_code, text)
        raws = []

        def keep_raw(m):
            raws.append(m.group(0))
            return f"\x01{len(raws) - 1}\x01"

        # Inline HTML the documents use on purpose.
        text = re.sub(r"<br\s*/?>|</?(?:kbd|sub|sup|b|i|em|strong)>", keep_raw, text)
        text = html.escape(text, quote=False)
        text = re.sub(r"&lt;(https?://[^\s&]+)&gt;", lambda m: f'<a href="{m.group(1)}">{m.group(1)}</a>', text)
        text = re.sub(r"!\[([^\]]*)\]\(([^)\s]+)(?:\s+&quot;[^)]*&quot;)?\)", lambda m: f'<img src="{self.link(html.unescape(m.group(2)))}" alt="{m.group(1)}">', text)
        text = re.sub(r"\[([^\]]+)\]\(([^)\s]+)\)", self.anchor_tag, text)
        text = re.sub(r"\*\*(.+?)\*\*", r"<strong>\1</strong>", text)
        text = re.sub(r"(?<![\w*])\*(?!\s)(.+?)(?<!\s)\*(?![\w*])", r"<em>\1</em>", text)
        text = re.sub(r"(?<![\w_])_(?!\s)(.+?)(?<!\s)_(?![\w_])", r"<em>\1</em>", text)
        text = re.sub(r"\x01(\d+)\x01", lambda m: raws[int(m.group(1))], text)
        return re.sub(r"\x00(\d+)\x00", lambda m: codes[int(m.group(1))], text)

    def convert(self, md, shift=0):
        """Markdown to HTML. shift lowers every heading level (a chapter's
        ## becomes its title)."""
        lines = md.replace("\t", "    ").split("\n")
        out = []
        i = 0
        para = []

        def flush():
            if para:
                out.append("<p>" + self.inline(" ".join(s.strip() for s in para)) + "</p>")
                para.clear()

        while i < len(lines):
            line = lines[i]
            s = line.strip()
            if not s:
                flush()
                i += 1
                continue
            m = re.match(r"^(\s*)(```+|~~~+)\s*([\w+-]*)", line)
            if m:
                flush()
                fence, lang = m.group(2), m.group(3)
                body = []
                i += 1
                while i < len(lines) and not lines[i].strip().startswith(fence):
                    body.append(lines[i][len(m.group(1)):] if lines[i].startswith(m.group(1)) else lines[i])
                    i += 1
                i += 1
                cls = f' class="lang-{lang}"' if lang else ""
                out.append(f"<pre><code{cls}>" + html.escape("\n".join(body)) + "</code></pre>")
                continue
            m = re.match(r"^(#{1,6})\s+(.*?)\s*#*\s*$", s)
            if m:
                flush()
                level = min(6, max(1, len(m.group(1)) - shift))
                text = m.group(2)
                aid = self.anchor(text)
                if level in (2, 3):
                    self.toc.append((level, aid, re.sub(r"<[^>]+>", "", self.inline(text))))
                out.append(f'<h{level} id="{aid}">{self.inline(text)}<a class="hash" href="#{aid}" aria-label="Link to this section">#</a></h{level}>')
                i += 1
                continue
            if re.match(r"^(-{3,}|\*{3,}|_{3,})$", s):
                flush()
                out.append("<hr>")
                i += 1
                continue
            if s.startswith("|") and i + 1 < len(lines) and re.match(r"^\|?\s*:?-{2,}", lines[i + 1].strip()):
                flush()
                i = self.table(lines, i, out)
                continue
            if s.startswith(">"):
                flush()
                quote = []
                while i < len(lines) and lines[i].strip().startswith(">"):
                    quote.append(re.sub(r"^\s*>\s?", "", lines[i]))
                    i += 1
                inner = Converter(self.link)
                inner.ids = self.ids
                out.append("<blockquote>" + inner.convert("\n".join(quote)) + "</blockquote>")
                continue
            if re.match(r"^\s*([-*+]|\d+[.)])\s+", line):
                flush()
                i = self.list(lines, i, out)
                continue
            if RAW_BLOCK.match(s) and not para:
                flush()
                block = []
                while i < len(lines) and lines[i].strip():
                    block.append(lines[i])
                    i += 1
                out.append("\n".join(block))
                continue
            para.append(line)
            i += 1
        flush()
        return "\n".join(out)

    def table(self, lines, i, out):
        def cells(row):
            row = row.strip()
            if row.startswith("|"):
                row = row[1:]
            if row.endswith("|") and not row.endswith("\\|"):
                row = row[:-1]
            parts, cur, code = [], "", False
            for ch in row:
                if ch == "`":
                    code = not code
                if ch == "|" and not code and not cur.endswith("\\"):
                    parts.append(cur)
                    cur = ""
                else:
                    cur += ch
            parts.append(cur)
            return [p.strip().replace("\\|", "|") for p in parts]

        head = cells(lines[i])
        aligns = []
        for c in cells(lines[i + 1]):
            aligns.append("right" if c.endswith(":") and not c.startswith(":") else "center" if c.startswith(":") and c.endswith(":") else "")
        i += 2
        rows = []
        while i < len(lines) and lines[i].strip().startswith("|"):
            rows.append(cells(lines[i]))
            i += 1

        def td(tag, text, k):
            a = aligns[k] if k < len(aligns) else ""
            style = f' style="text-align:{a}"' if a else ""
            return f"<{tag}{style}>{self.inline(text)}</{tag}>"

        t = ['<div class="table"><table><thead><tr>' + "".join(td("th", c, k) for k, c in enumerate(head)) + "</tr></thead><tbody>"]
        for r in rows:
            t.append("<tr>" + "".join(td("td", c, k) for k, c in enumerate(r)) + "</tr>")
        t.append("</tbody></table></div>")
        out.append("".join(t))
        return i

    def list(self, lines, i, out):
        """A list, with nesting by indentation and items that go on over
        several lines."""
        def marker(line):
            return re.match(r"^(\s*)([-*+]|\d+[.)])\s+(.*)$", line)

        first = marker(lines[i])
        indent = len(first.group(1))
        ordered = first.group(2)[0].isdigit()
        start = int(re.match(r"\d+", first.group(2)).group()) if ordered else 1
        items = []
        while i < len(lines):
            line = lines[i]
            m = marker(line)
            if m and len(m.group(1)) == indent and (m.group(2)[0].isdigit()) == ordered:
                items.append([m.group(3)])
                i += 1
                continue
            if not line.strip():
                # A blank line ends the list unless an indented line follows.
                nxt = lines[i + 1] if i + 1 < len(lines) else ""
                if nxt.strip() and (len(nxt) - len(nxt.lstrip())) > indent and items:
                    items[-1].append("")
                    i += 1
                    continue
                break
            if (len(line) - len(line.lstrip())) > indent and items:
                items[-1].append(line)
                i += 1
                continue
            if m is None and items and not re.match(r"^\s*(#|\||>|```)", line):
                # A lazy continuation line of the item's paragraph.
                items[-1].append(line)
                i += 1
                continue
            break
        tag = "ol" if ordered else "ul"
        attr = f' start="{start}"' if ordered and start != 1 else ""
        html_items = []
        for it in items:
            head, rest = it[0], it[1:]
            # The item's own text is its first line plus the lines up to a nested block.
            text = [head]
            j = 0
            while j < len(rest) and rest[j].strip() and not marker(rest[j]) and not rest[j].strip().startswith(("```", "|")):
                text.append(rest[j].strip())
                j += 1
            body = self.inline(" ".join(text))
            tail = rest[j:]
            if any(x.strip() for x in tail):
                strip = min((len(x) - len(x.lstrip()) for x in tail if x.strip()), default=0)
                inner = Converter(self.link)
                inner.ids = self.ids
                body += inner.convert("\n".join(x[strip:] for x in tail))
            html_items.append(f"<li>{body}</li>")
        out.append(f"<{tag}{attr}>" + "".join(html_items) + f"</{tag}>")
        return i


# ---------- Pages ----------

def read(path):
    with open(os.path.join(ROOT, path), encoding="utf-8") as f:
        return f.read()


def split_guide(md):
    """The guide's intro, and its ## sections by title."""
    parts = re.split(r"(?m)^## ", md)
    intro = re.sub(r"(?m)^# .*\n", "", parts[0]).strip()
    sections = {}
    order = []
    for p in parts[1:]:
        title, _, body = p.partition("\n")
        sections[title.strip()] = body
        order.append(title.strip())
    return intro, sections, order


TITLES = {}  # page file -> title, for links written as file names


def main():
    for _, entries in SECTIONS:
        for slug, _, title, _ in entries:
            TITLES[f"{slug}.html"] = title
    out_dir = sys.argv[1] if len(sys.argv) > 1 else os.path.join(ROOT, "site", "docs")
    os.makedirs(out_dir, exist_ok=True)

    pages = {}  # slug -> (title, markdown, shift)
    source_of = {}  # repo path -> slug
    for _, entries in SECTIONS:
        for slug, src, title, _ in entries:
            if src and src != "guide":
                source_of[src] = slug

    intro, sections, order = split_guide(read("docs/USER_GUIDE.md"))
    used = set()
    guide_pages = []
    for slug, title, summary, names in GUIDE:
        body = []
        for n in names:
            if n in sections:
                body.append(f"## {n}\n{sections[n]}")
                used.add(n)
        guide_pages.append((slug, title, summary, "\n".join(body)))
    rest = [n for n in order if n not in used]
    if rest:
        # A section added to the guide but not to a chapter still shows.
        guide_pages.append(("guide-more", "More", "Everything else in the guide.", "\n".join(f"## {n}\n{sections[n]}" for n in rest)))
        SECTIONS[0][1].append(("guide-more", "guide", "More", "Everything else in the guide."))

    # Where every heading ends up, so #anchors in other documents follow.
    anchor_page = {}
    for slug, _, _, body in guide_pages:
        for h in re.findall(r"(?m)^#{2,4}\s+(.*)$", body):
            anchor_page.setdefault(slugify(h), slug)
    source_of["docs/USER_GUIDE.md"] = "guide"

    def linker(src_path):
        base = os.path.dirname(src_path)

        def link(target):
            if re.match(r"^[a-z]+:", target) or target.startswith("//"):
                return target
            path, _, frag = target.partition("#")
            if not path:
                return "#" + frag
            full = os.path.normpath(os.path.join(base, path)).replace(os.sep, "/")
            if full == "docs/USER_GUIDE.md" and frag in anchor_page:
                return f"{anchor_page[frag]}.html#{frag}"
            if full in source_of:
                return f"{source_of[full]}.html" + (f"#{frag}" if frag else "")
            if full.startswith("docs/") and full.endswith("/README.md") or full == "docs/README.md" or full == "docs":
                return "index.html"
            kind = "tree" if not os.path.splitext(full)[1] else "blob"
            return f"{REPO}/{kind}/main/{full}" + (f"#{frag}" if frag else "")

        return link

    nav_html = sidebar_html()

    def write(slug, title, body_html, toc, summary=""):
        page = TEMPLATE.format(
            title=html.escape(title), summary=html.escape(summary or title),
            nav=nav_html.replace(f'href="{slug}.html"', f'href="{slug}.html" aria-current="page"'),
            body=body_html, toc=toc_html(toc))
        with open(os.path.join(out_dir, f"{slug}.html"), "w", encoding="utf-8") as f:
            f.write(page)

    for _, entries in SECTIONS:
        for slug, src, title, summary in entries:
            if not src or src == "guide":
                continue
            conv = Converter(linker(src))
            md = read(src)
            write(slug, title, conv.convert(md), conv.toc, summary)

    conv = Converter(linker("docs/USER_GUIDE.md"))
    cards = "".join(f'<a class="doc-card" href="{s}.html"><b>{html.escape(t)}</b><span>{html.escape(d)}</span></a>' for s, t, d, _ in guide_pages)
    write("guide", "User guide", "<h1>User guide</h1>" + conv.convert(intro) + f'<div class="doc-cards">{cards}</div>', [], "Everything you can do with Pimpo, screen by screen.")
    for slug, title, summary, body in guide_pages:
        conv = Converter(linker("docs/USER_GUIDE.md"))
        write(slug, title, f'<p class="crumb"><a href="guide.html">User guide</a></p><h1>{html.escape(title)}</h1><p class="lede">{html.escape(summary)}</p>' + conv.convert(body), conv.toc, summary)

    groups = []
    for name, entries in SECTIONS:
        items = "".join(f'<a class="doc-card" href="{s}.html"><b>{html.escape(t)}</b><span>{inline_plain(d)}</span></a>' for s, _, t, d in entries if s != "index")
        groups.append(f'<section class="doc-group"><h2 id="{slugify(name)}">{html.escape(name)}</h2><div class="doc-cards">{items}</div></section>')
    write("index", "Documentation",
          '<h1>Documentation</h1><p class="lede">Everything about Pimpo, from the first task to writing a connector. New here? Start with <a href="guide-start.html">Getting started</a>.</p>'
          + "".join(groups), [(2, slugify(n), n) for n, _ in SECTIONS], "Everything about Pimpo, from the first task to writing a connector.")
    problems = check(out_dir)
    for p in problems:
        print("broken:", p, file=sys.stderr)
    print(f"wrote {sum(1 for _ in os.listdir(out_dir) if _.endswith('.html'))} pages to {out_dir}")
    if problems:
        sys.exit(1)


def check(out_dir):
    """Links between the pages, and the #anchors they point at, must exist."""
    problems = []
    ids = {}
    for f in os.listdir(out_dir):
        if f.endswith(".html"):
            with open(os.path.join(out_dir, f), encoding="utf-8") as h:
                text = h.read()
            ids[f] = (set(re.findall(r'id="([^"]+)"', text)), text)
    for f, (_, text) in ids.items():
        for href in re.findall(r'href="([^"]+)"', text):
            if re.match(r"^[a-z]+:|^//|^\.\./", href):
                continue
            page, _, frag = href.partition("#")
            page = page or f
            if page not in ids:
                problems.append(f"{f}: {href}")
            elif frag and frag not in ids[page][0]:
                problems.append(f"{f}: {href}")
    return problems


def inline_plain(text):
    return re.sub(r"`([^`]+)`", r"<code>\1</code>", html.escape(text))


def sidebar_html():
    parts = []
    for name, entries in SECTIONS:
        links = []
        for slug, src, title, _ in entries:
            cls = ' class="sub"' if src == "guide" else ""
            links.append(f'<li{cls}><a href="{slug}.html">{html.escape(title)}</a></li>')
        parts.append(f'<div class="nav-group"><p>{html.escape(name)}</p><ul>{"".join(links)}</ul></div>')
    return "".join(parts)


def toc_html(toc):
    if not toc:
        return ""
    items = "".join(f'<li class="l{lvl}"><a href="#{aid}">{text}</a></li>' for lvl, aid, text in toc)
    return f'<nav class="toc" aria-label="On this page"><p>On this page</p><ul>{items}</ul></nav>'


TEMPLATE = """<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{title} · Pimpo docs</title>
<meta name="description" content="{summary}">
<link rel="icon" href="../favicon.svg" type="image/svg+xml">
<link rel="stylesheet" href="../docs.css">
</head>
<body>
<header class="top">
  <a class="brand" href="../"><img src="../favicon.svg" alt="">Pimpo</a>
  <nav class="top-nav">
    <a href="../#features">Features</a>
    <a href="../#download">Download</a>
    <a href="index.html">Docs</a>
    <a href="https://github.com/turbine-dev/pimpo">GitHub</a>
  </nav>
</header>
<div class="layout">
  <details class="side" open>
    <summary>Documentation</summary>
    <label class="find"><span class="sr">Find a page</span><input type="search" placeholder="Find a page…" autocomplete="off"></label>
    <nav aria-label="Documentation">{nav}</nav>
  </details>
  <main class="doc">
{body}
  <p class="edit">This page is built from the repository's documentation. <a href="https://github.com/turbine-dev/pimpo/tree/main/docs">Improve it on GitHub</a>.</p>
  </main>
  {toc}
</div>
<script>
// Small screens start with the page list closed; the filter narrows it.
if (matchMedia('(max-width: 900px)').matches) document.querySelector('.side').removeAttribute('open')
var box = document.querySelector('.find input')
box.addEventListener('input', function () {{
  var q = box.value.trim().toLowerCase()
  document.querySelectorAll('.side li').forEach(function (li) {{ li.hidden = q && li.textContent.toLowerCase().indexOf(q) < 0 }})
  document.querySelectorAll('.nav-group').forEach(function (g) {{ g.hidden = !g.querySelector('li:not([hidden])') }})
}})
</script>
</body>
</html>
"""


if __name__ == "__main__":
    main()
