// Compares the tools on this machine with the versions pinned in the repository:
// the Go toolchain in go.mod, Node in .nvmrc and the Wails CLI in Taskfile.yml (passed
// as the first argument by `task versions`). Prints one line per tool and exits with 1
// when any of them differs, so `task setup` stops before the hooks are installed.
import { spawnSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL("..", import.meta.url));

function read(name) {
  return readFileSync(root + name, "utf8");
}

function run(command, args) {
  const result = spawnSync(command, args, { cwd: root, encoding: "utf8" });
  if (result.error || result.status !== 0) {
    return "not found";
  }
  // Some tools print their version on stderr. Drop colour codes and keep the first
  // version-looking word.
  const plain = (result.stdout + result.stderr).replace(/\u001b\[[0-9;]*m/g, "");
  return plain.match(/(go)?v?\d+\.\d+\.\d+/)?.[0] ?? (plain.trim() || "no version printed");
}

const wanted = {
  go: read("go.mod").match(/^toolchain (go\S+)\s*$/m)?.[1] ?? "missing in go.mod",
  node: "v" + read(".nvmrc").trim(),
  wails3: process.argv[2] ?? "missing argument",
};

const found = {
  go: run("go", ["env", "GOVERSION"]),
  node: process.version,
  wails3: run("wails3", ["version"]),
};

let differences = 0;
for (const tool of Object.keys(wanted)) {
  const same = wanted[tool] === found[tool];
  if (!same) differences += 1;
  console.log(
    `${same ? "ok  " : "DIFF"} ${tool.padEnd(7)} pinned ${wanted[tool].padEnd(10)} found ${found[tool]}`,
  );
}

if (differences > 0) {
  console.log(
    "Install the pinned versions: Go from https://go.dev/dl/, Node from .nvmrc, " +
      "and the Wails CLI with `task setup`.",
  );
  process.exit(1);
}
