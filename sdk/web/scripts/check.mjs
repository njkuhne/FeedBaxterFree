// Post-build checks for the web SDK: size budget, IIFE smoke, source rules. Node standard library only.
import { existsSync, readdirSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { gzipSync } from "node:zlib";
import vm from "node:vm";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const BUDGET = 4096;

/** @type {string[]} */
const failures = [];

for (const f of ["dist/index.js", "dist/index.d.ts", "dist/feedbaxter.js"]) {
  if (!existsSync(join(root, f))) failures.push(`missing ${f}`);
}

const iifePath = join(root, "dist", "feedbaxter.js");
if (existsSync(iifePath)) {
  const raw = readFileSync(iifePath);
  const gz = gzipSync(raw, { level: 9 }).length;
  console.log(`feedbaxter.js: ${raw.length} bytes, ${gz} gzipped (budget ${BUDGET})`);
  if (gz > BUDGET) failures.push(`feedbaxter.js is ${gz} bytes gzipped, budget ${BUDGET}`);

  const ctx = vm.createContext({});
  ctx.window = ctx;
  try {
    vm.runInContext(raw.toString("utf8"), ctx);
    const fb = vm.runInContext("typeof FeedBaxter === 'undefined' ? undefined : FeedBaxter", ctx);
    for (const name of ["createClient", "init", "send"]) {
      if (!fb || typeof fb[name] !== "function") failures.push(`IIFE: FeedBaxter.${name} is not a function`);
    }
    if (fb && typeof fb.send === "function") {
      let msg = "";
      try {
        fb.send({ message: "x" });
        msg = "did not throw";
      } catch (e) {
        msg = e && typeof e === "object" && "message" in e ? String(e.message) : "";
      }
      if (!msg.includes("init")) failures.push(`IIFE: send before init should throw an error mentioning init, got: ${msg}`);
    }
  } catch (e) {
    failures.push(`IIFE smoke failed: ${e}`);
  }
}

const srcDir = join(root, "src");
const banned = [
  [/document\./, "document."],
  [/navigator\./, "navigator."],
  [/location\./, "location."],
  [/cookie/, "cookie"],
  [/innerHTML/, "innerHTML"],
  [/console\./, "console."],
  [/:\s*any\b|as any\b/, "any type"],
  [/https?:\/\//, "URL literal"],
];
for (const name of readdirSync(srcDir)) {
  if (!name.endsWith(".ts") || name.endsWith(".test.ts")) continue;
  readFileSync(join(srcDir, name), "utf8")
    .split("\n")
    .forEach((line, i) => {
      for (const [re, label] of banned) if (re.test(line)) failures.push(`src/${name}:${i + 1}: ${label}`);
    });
}

const pkg = JSON.parse(readFileSync(join(root, "package.json"), "utf8"));
if (pkg.dependencies && Object.keys(pkg.dependencies).length > 0) failures.push("package.json has dependencies");
if ("private" in pkg) failures.push("package.json has a private key");

if (failures.length > 0) {
  console.error(failures.join("\n"));
  console.error(`\ncheck failed: ${failures.length} problem(s)`);
  process.exit(1);
}
console.log("check passed");
