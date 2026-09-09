import { readFile, stat, writeFile } from "node:fs/promises";
import path from "node:path";

const crcTable = Array.from({ length: 256 }, (_, index) => {
  let value = index;
  for (let bit = 0; bit < 8; bit += 1) value = value & 1 ? 0xedb88320 ^ (value >>> 1) : value >>> 1;
  return value >>> 0;
});

function crc32(data) {
  let value = 0xffffffff;
  for (const byte of data) value = crcTable[(value ^ byte) & 0xff] ^ (value >>> 8);
  return (value ^ 0xffffffff) >>> 0;
}

function localHeader(name, data, checksum) {
  const header = Buffer.alloc(30);
  header.writeUInt32LE(0x04034b50, 0);
  header.writeUInt16LE(20, 4);
  header.writeUInt16LE(0, 6);
  header.writeUInt16LE(0, 8);
  header.writeUInt16LE(0, 10);
  header.writeUInt16LE(0x21, 12);
  header.writeUInt32LE(checksum, 14);
  header.writeUInt32LE(data.length, 18);
  header.writeUInt32LE(data.length, 22);
  header.writeUInt16LE(name.length, 26);
  return header;
}

function centralHeader(name, data, checksum, offset, mode) {
  const header = Buffer.alloc(46);
  header.writeUInt32LE(0x02014b50, 0);
  header.writeUInt16LE(0x0314, 4);
  header.writeUInt16LE(20, 6);
  header.writeUInt16LE(0, 8);
  header.writeUInt16LE(0, 10);
  header.writeUInt16LE(0, 12);
  header.writeUInt16LE(0x21, 14);
  header.writeUInt32LE(checksum, 16);
  header.writeUInt32LE(data.length, 20);
  header.writeUInt32LE(data.length, 24);
  header.writeUInt16LE(name.length, 28);
  header.writeUInt32LE(((mode & 0xffff) << 16) >>> 0, 38);
  header.writeUInt32LE(offset, 42);
  return header;
}

/** Write a deterministic stored ZIP. It is also used by defect fixtures. */
export async function writeZip(file, inputEntries) {
  const entries = inputEntries.map((entry) => ({
    name: entry.name,
    data: Buffer.from(entry.data),
    mode: entry.mode ?? 0o100644,
  })).sort((left, right) => left.name.localeCompare(right.name));
  const local = [];
  const central = [];
  let offset = 0;
  for (const entry of entries) {
    const name = Buffer.from(entry.name);
    const checksum = crc32(entry.data);
    const header = localHeader(name, entry.data, checksum);
    local.push(header, name, entry.data);
    central.push(centralHeader(name, entry.data, checksum, offset, entry.mode), name);
    offset += header.length + name.length + entry.data.length;
  }
  const centralSize = central.reduce((size, buffer) => size + buffer.length, 0);
  const end = Buffer.alloc(22);
  end.writeUInt32LE(0x06054b50, 0);
  end.writeUInt16LE(entries.length, 8);
  end.writeUInt16LE(entries.length, 10);
  end.writeUInt32LE(centralSize, 12);
  end.writeUInt32LE(offset, 16);
  await writeFile(file, Buffer.concat([...local, ...central, end]));
}

/** Read stored ZIP members from the central directory and verify their CRC values. */
export async function readZip(file) {
  const archive = await readFile(file);
  const endOffset = archive.lastIndexOf(Buffer.from([0x50, 0x4b, 0x05, 0x06]));
  if (endOffset < 0 || endOffset + 22 > archive.length) throw new Error("ZIP end record is missing.");
  const count = archive.readUInt16LE(endOffset + 10);
  let cursor = archive.readUInt32LE(endOffset + 16);
  const entries = [];
  for (let index = 0; index < count; index += 1) {
    if (archive.readUInt32LE(cursor) !== 0x02014b50) throw new Error("ZIP central entry is invalid.");
    const method = archive.readUInt16LE(cursor + 10);
    const checksum = archive.readUInt32LE(cursor + 16);
    const compressedSize = archive.readUInt32LE(cursor + 20);
    const size = archive.readUInt32LE(cursor + 24);
    const nameLength = archive.readUInt16LE(cursor + 28);
    const extraLength = archive.readUInt16LE(cursor + 30);
    const commentLength = archive.readUInt16LE(cursor + 32);
    const mode = archive.readUInt32LE(cursor + 38) >>> 16;
    const localOffset = archive.readUInt32LE(cursor + 42);
    const name = archive.subarray(cursor + 46, cursor + 46 + nameLength).toString("utf8");
    if (method !== 0) throw new Error(`Unsupported ZIP compression method ${method}.`);
    if (archive.readUInt32LE(localOffset) !== 0x04034b50) throw new Error("ZIP local entry is invalid.");
    const localNameLength = archive.readUInt16LE(localOffset + 26);
    const localExtraLength = archive.readUInt16LE(localOffset + 28);
    const start = localOffset + 30 + localNameLength + localExtraLength;
    const data = archive.subarray(start, start + compressedSize);
    if (data.length !== size || crc32(data) !== checksum) throw new Error(`ZIP member checksum failed: ${name}`);
    entries.push({ name, data: Buffer.from(data), mode });
    cursor += 46 + nameLength + extraLength + commentLength;
  }
  return entries;
}

async function manifestAt(sourceRoot) {
  return JSON.parse(await readFile(path.join(sourceRoot, "hooks", "manifest.json"), "utf8"));
}

async function expectedEntries(sourceRoot, manifest, sourceRevision) {
  const entries = [];
  for (const file of manifest.files.toSorted()) {
    const source = path.join(sourceRoot, file);
    entries.push({
      name: `${manifest.artifacts.prefix}${file}`,
      data: await readFile(source),
      mode: (await stat(source)).mode,
    });
  }
  entries.push({
    name: manifest.artifacts.metadata,
    data: Buffer.from(`${JSON.stringify({
      name: manifest.name,
      version: manifest.version,
      hosts: ["codex", "claude-code"],
      repository: manifest.repository,
      releaseTag: `v${manifest.version}`,
      releaseUrl: `${manifest.repository}/releases/tag/v${manifest.version}`,
      updateUrl: `${manifest.repository}/releases/latest`,
      sourceRevision,
    }, null, 2)}\n`),
  });
  return entries.sort((left, right) => left.name.localeCompare(right.name));
}

/** Build one reproducible archive containing the complete cross-host consumer package. */
export async function buildArtifacts({ sourceRoot, outputDirectory, sourceRevision }) {
  const manifest = await manifestAt(sourceRoot);
  const file = path.join(outputDirectory, `agent-team-${manifest.version}.zip`);
  await writeZip(file, await expectedEntries(sourceRoot, manifest, sourceRevision));
  const archives = [file];
  return { status: "built", sourceRevision, archives };
}

function unsafe(name) {
  const normalized = name.replaceAll("\\", "/");
  return normalized.startsWith("/") || /^[a-z]:\//i.test(normalized) || normalized.split("/").includes("..") || normalized.includes("\0");
}

/** Compare supplied archives with the manifest, source bytes, platform, and source revision. */
export async function checkArtifacts({ sourceRoot, archives = [], expectedRevision }) {
  if (!archives.length) return { status: "not_applicable", reason: "no_archive", errors: [] };
  const manifest = await manifestAt(sourceRoot);
  const errors = [];
  if (archives.length !== 1) errors.push(`Expected one universal archive, received ${archives.length}.`);
  for (const archive of archives) {
    let entries;
    try {
      entries = await readZip(archive);
    } catch (error) {
      errors.push(`${path.basename(archive)}: ${error.message}`);
      continue;
    }
    const names = entries.map(({ name }) => name);
    for (const name of names) if (unsafe(name)) errors.push(`${path.basename(archive)} has an unsafe path: ${name}`);
    for (const name of new Set(names)) if (names.filter((entry) => entry === name).length > 1) errors.push(`${path.basename(archive)} has a duplicate entry: ${name}`);
    const metadataEntry = entries.find(({ name }) => name === manifest.artifacts.metadata);
    let metadata;
    try {
      metadata = JSON.parse(metadataEntry?.data.toString("utf8") ?? "");
    } catch {
      errors.push(`${path.basename(archive)} has invalid source metadata.`);
      continue;
    }
    if (JSON.stringify(metadata.hosts) !== JSON.stringify(["codex", "claude-code"])) errors.push(`${path.basename(archive)} does not identify both supported hosts.`);
    if (metadata.sourceRevision !== expectedRevision) errors.push(`${path.basename(archive)} has stale source revision metadata.`);
    if (metadata.version !== manifest.version) errors.push(`${path.basename(archive)} has stale version metadata.`);
    if (metadata.repository !== manifest.repository) errors.push(`${path.basename(archive)} has stale repository metadata.`);
    if (metadata.releaseTag !== `v${manifest.version}`) errors.push(`${path.basename(archive)} has stale release tag metadata.`);
    const expected = await expectedEntries(sourceRoot, manifest, expectedRevision);
    const expectedMap = new Map(expected.map((entry) => [entry.name, entry]));
    const actualMap = new Map(entries.map((entry) => [entry.name, entry]));
    for (const [name, expectedEntry] of expectedMap) {
      if (!actualMap.has(name)) errors.push(`${path.basename(archive)} omits ${name}.`);
      else if (!actualMap.get(name).data.equals(expectedEntry.data)) errors.push(`${path.basename(archive)} has stale content for ${name}.`);
      else if (expectedEntry.mode !== undefined && (actualMap.get(name).mode & 0o777) !== (expectedEntry.mode & 0o777)) errors.push(`${path.basename(archive)} has stale mode for ${name}.`);
    }
    for (const name of actualMap.keys()) if (!expectedMap.has(name)) errors.push(`${path.basename(archive)} has unexpected entry ${name}.`);
  }
  return { status: errors.length ? "failed" : "passed", errors };
}
