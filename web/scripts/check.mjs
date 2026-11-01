// Source rules for web/src (see docs/design/m3-ui-design.md section 0). Node standard library only.
import { readdirSync, readFileSync, statSync } from "node:fs";
import { dirname, join, relative, sep } from "node:path";
import { fileURLToPath } from "node:url";

const root = join(dirname(fileURLToPath(import.meta.url)), "..", "src");
const TOKENS = ["styles", "tokens.css"].join(sep);
const FONT_FILES = new Set([TOKENS, join("styles", "base.css"), join("styles", "fonts.css")]);

/** @type {string[]} */
const failures = [];

function walk(dir) {
  for (const name of readdirSync(dir)) {
    const full = join(dir, name);
    if (statSync(full).isDirectory()) walk(full);
    else if (/\.(ts|tsx|css|html)$/.test(name)) check(full);
  }
}

function fail(rel, line, rule) {
  failures.push(`${rel}:${line}: ${rule}`);
}

function check(full) {
  const rel = relative(join(root, ".."), full);
  const inSrc = relative(root, full);
  const isTest = /\.test\.(ts|tsx)$/.test(full);
  const isCss = full.endsWith(".css");
  const lines = readFileSync(full, "utf8").split("\n");
  lines.forEach((text, i) => {
    const n = i + 1;
    // The SVG namespace string in a data URI is not a request.
    const noNs = text.replaceAll("http://www.w3.org/2000/svg", "");
    if (inSrc !== TOKENS) {
      if (/#[0-9a-fA-F]{3,8}\b/.test(noNs) && !/&#/.test(noNs) && !/href=["']#/.test(noNs) && isColourContext(noNs, isCss)) {
        fail(rel, n, "raw hex colour outside styles/tokens.css");
      }
      if (/\b(rgba?|hsla?)\(/.test(text)) fail(rel, n, "rgb()/rgba()/hsl() outside styles/tokens.css");
    }
    if (/dangerouslySetInnerHTML/.test(text)) fail(rel, n, "dangerouslySetInnerHTML");
    if (/innerHTML/.test(text)) fail(rel, n, "innerHTML");
    if (/:\s*any\b|as any\b|<any>/.test(text)) fail(rel, n, "any type");
    if (!isTest && /https?:\/\//.test(noNs)) fail(rel, n, "external URL");
    if (!FONT_FILES.has(inSrc) && /font-family\s*:/.test(text) && !/font-family\s*:\s*var\(--font-/.test(text)) {
      fail(rel, n, "font-family that is not var(--font-...)");
    }
    if (/outline\s*:\s*(none|0)\b/.test(text)) fail(rel, n, "outline removed");
  });
}

// A "#abc" in TS/TSX is usually an href fragment or text; only flag it where it sits in a string or CSS value.
function isColourContext(text, isCss) {
  if (isCss) return true;
  return /["'`]#[0-9a-fA-F]{3,8}["'`]/.test(text);
}

walk(root);
if (failures.length > 0) {
  console.error(failures.join("\n"));
  console.error(`\ncheck failed: ${failures.length} problem(s)`);
  process.exit(1);
}
console.log("check passed");
