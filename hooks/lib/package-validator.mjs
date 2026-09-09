import { access, readFile, readdir, stat } from "node:fs/promises";
import path from "node:path";

async function present(file) {
  try {
    await access(file);
    return true;
  } catch {
    return false;
  }
}

async function filesUnder(root, relative, output = []) {
  const directory = path.join(root, relative);
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    const name = path.posix.join(relative.replaceAll("\\", "/"), entry.name);
    if (entry.isDirectory()) await filesUnder(root, name, output);
    else if (entry.isFile()) output.push(name);
  }
  return output;
}

function localLinks(source) {
  return [...source.matchAll(/!?\[[^\]]*\]\(([^)]+)\)/g)].map((match) => match[1].trim().replace(/^<|>$/g, ""));
}

async function validatePackage(root, { source }) {
  const errors = [];
  let manifest;
  try {
    manifest = JSON.parse(await readFile(path.join(root, "hooks", "manifest.json"), "utf8"));
  } catch (error) {
    return { status: "failed", errors: [`Cannot read hooks/manifest.json: ${error.message}`] };
  }

  for (const runtime of ["codex", "claude"]) {
    if (!manifest.runtimeDeclarations?.[runtime]) errors.push(`Universal package manifest is missing the ${runtime} adapter declaration.`);
    if (!manifest.requiredEvents?.[runtime]?.length) errors.push(`Universal package manifest is missing ${runtime} required events.`);
    if (!manifest.registrationTargets?.[runtime]?.user || !manifest.registrationTargets?.[runtime]?.project) errors.push(`Universal package manifest is missing ${runtime} user/project registration targets.`);
  }

  for (const file of manifest.files) if (!(await present(path.join(root, file)))) errors.push(`Missing manifest file: ${file}`);
  if (source) {
    if (!(await present(path.join(root, manifest.legacy.marker)))) errors.push(`Missing legacy archive marker: ${manifest.legacy.marker}`);
    if (manifest.files.some((file) => file.startsWith(`${manifest.legacy.path}/`))) errors.push("Legacy files must not be in the current package manifest.");

    const discovered = [...manifest.rootFiles];
    for (const directory of manifest.includeRoots) {
      if (await present(path.join(root, directory))) await filesUnder(root, directory, discovered);
    }
    for (const file of discovered) if (!manifest.files.includes(file)) errors.push(`Current package file is absent from manifest: ${file}`);
    for (const file of manifest.files) if (!discovered.includes(file)) errors.push(`Manifest path is outside current package roots: ${file}`);
  }

  const skill = await readFile(path.join(root, "SKILL.md"), "utf8").catch(() => "");
  const readme = await readFile(path.join(root, "README.md"), "utf8").catch(() => "");
  const changelog = await readFile(path.join(root, "CHANGELOG.md"), "utf8").catch(() => "");
  const skillVersion = skill.match(/^\s*version:\s*["']?([^"'\s]+)["']?\s*$/m)?.[1];
  const readmeVersion = readme.match(/current skill version is \*\*([^*]+)\*\*/i)?.[1];
  if (skillVersion !== manifest.version) errors.push(`SKILL.md version ${skillVersion ?? "missing"} does not match manifest ${manifest.version}.`);
  if (readmeVersion !== manifest.version) errors.push(`README.md version ${readmeVersion ?? "missing"} does not match manifest ${manifest.version}.`);
  if (changelog.match(/^##\s+([^\s]+)\s+-/m)?.[1] !== manifest.version) errors.push(`CHANGELOG.md latest version does not match manifest ${manifest.version}.`);
  if (manifest.repository !== "https://github.com/thebpandey/agent-team") errors.push("Manifest repository is not the canonical Agent-Team source.");

  for (const file of manifest.files.filter((entry) => entry.endsWith(".md"))) {
    let source;
    try {
      source = await readFile(path.join(root, file), "utf8");
    } catch {
      continue;
    }
    for (const target of localLinks(source)) {
      if (/^(?:[a-z]+:|#)/i.test(target)) continue;
      const clean = decodeURIComponent(target.split("#")[0].split("?")[0]);
      const resolved = path.resolve(path.dirname(path.join(root, file)), clean);
      if (!resolved.startsWith(`${path.resolve(root)}${path.sep}`) || !(await present(resolved))) errors.push(`Broken local link in ${file}: ${target}`);
    }
  }

  for (const entrypoint of manifest.entrypoints) {
    try {
      if (!((await stat(path.join(root, entrypoint))).mode & 0o111)) errors.push(`Entrypoint is not executable: ${entrypoint}`);
    } catch {
      // Missing files already have a more direct error.
    }
  }

  for (const [runtime, declarationPath] of Object.entries(manifest.runtimeDeclarations)) {
    let declaration;
    try {
      declaration = JSON.parse(await readFile(path.join(root, declarationPath), "utf8"));
    } catch {
      errors.push(`Missing or invalid ${runtime} hook adapter: ${declarationPath}`);
      continue;
    }
    for (const event of manifest.requiredEvents[runtime]) {
      const groups = declaration.hooks?.[event];
      if (!groups?.length) errors.push(`${runtime} hook registration is missing ${event}.`);
      else if (!groups.some((group) => group.hooks?.some(({ command = "" }) => command.includes(`--runtime ${runtime}`) && command.includes(`--event ${event}`)))) {
        errors.push(`${runtime} ${event} does not target the shared adapter.`);
      }
    }
  }

  const ids = new Set();
  for (const policy of manifest.policies) {
    if (ids.has(policy.id)) errors.push(`Duplicate policy ID: ${policy.id}`);
    ids.add(policy.id);
    if ([...policy.platforms].sort().join(",") !== "claude,codex") errors.push(`Policy ${policy.id} lacks cross-runtime parity.`);
    if (!manifest.files.includes(policy.module) || !(await present(path.join(root, policy.module)))) errors.push(`Policy ${policy.id} has a missing module: ${policy.module}`);
  }
  for (let index = 1; index <= 15; index += 1) {
    const id = `req-${String(index).padStart(2, "0")}`;
    if (!ids.has(id)) errors.push(`Missing policy parity ID: ${id}`);
  }

  if (source) {
    for (const file of [".github/workflows/check-package.yml", ".github/workflows/release.yml", "tests/hooks-package.test.mjs", "tests/hooks-artifacts.test.mjs"]) {
      if (!(await present(path.join(root, file)))) errors.push(`Missing validation support file: ${file}`);
    }
  }
  return { status: errors.length ? "failed" : "passed", errors };
}

/** Check the source checkout, including source-only archive, test, and CI support. */
export async function checkPackage(root) {
  return validatePackage(root, { source: true });
}

/** Check only the files and behavior promised to an extracted package consumer. */
export async function checkInstalledPackage(root) {
  return validatePackage(root, { source: false });
}
