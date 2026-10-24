'use strict';

const fs = require('node:fs');
const path = require('node:path');
const { createRequire } = require('node:module');
const { pathToFileURL } = require('node:url');

function failure(code, message) {
  return Object.assign(new Error(message), { code });
}

function inside(root, file) {
  const relative = path.relative(root, fs.realpathSync(file));
  return relative !== '..' && !relative.startsWith(`..${path.sep}`) && !path.isAbsolute(relative);
}

function metadata(entry, id) {
  for (let directory = path.dirname(entry); ; directory = path.dirname(directory)) {
    const file = path.join(directory, 'package.json');
    if (fs.existsSync(file)) {
      const pkg = JSON.parse(fs.readFileSync(file, 'utf8'));
      if (pkg.name === id) return pkg;
    }
    if (directory === path.dirname(directory)) throw failure('dependencies_missing', `Cannot identify ${id}`);
  }
}

function library(request, config) {
  const directories = [];
  for (let directory = path.dirname(config); ; directory = path.dirname(directory)) {
    directories.push(directory);
    if (directory === request.root) break;
    if (directory === path.dirname(directory)) throw failure('config_invalid', 'Config is outside the project');
  }
  directories.push(path.resolve(request.root, request.composer_root));
  if (request.id !== 'jscpd') {
    for (const directory of [...new Set(directories)]) {
      const pkgFile = path.join(directory, 'package.json');
      const packagePath = path.join(directory, 'node_modules', request.id, 'package.json');
      let declared = false;
      if (fs.existsSync(pkgFile)) {
        const pkg = JSON.parse(fs.readFileSync(pkgFile, 'utf8'));
        declared = ['dependencies', 'devDependencies', 'peerDependencies', 'optionalDependencies']
          .some((key) => Object.hasOwn(pkg[key] || {}, request.id));
      }
      if (fs.existsSync(packagePath)) {
        if (!inside(request.root, packagePath)) throw failure('dependencies_missing', `${request.id} package is outside the mounted project`);
        const entry = createRequire(packagePath).resolve(request.id);
        if (!inside(request.root, entry)) throw failure('dependencies_missing', `${request.id} entry is outside the mounted project`);
        return { entry, pkg: metadata(entry, request.id) };
      }
      if (declared) {
        const resolver = createRequire(pkgFile);
        try {
          const entry = resolver.resolve(request.id);
          if (inside(request.root, entry)) return { entry, pkg: metadata(entry, request.id) };
        } catch (error) {
          if (error.code !== 'MODULE_NOT_FOUND') throw error;
        }
        throw failure('dependencies_missing', `Install the declared ${request.id} dependencies in ${directory}`);
      }
    }
  }
  try {
    const entry = createRequire(path.join(request.bundle, 'package.json')).resolve(request.id);
    return { entry, pkg: metadata(entry, request.id) };
  } catch (error) {
    if (error.code === 'MODULE_NOT_FOUND') throw failure('dependencies_missing', `Bundled ${request.id} is unavailable; reinstall the toolset`);
    throw error;
  }
}

function row(rule, text, severity, line, column, endLine, syntax = false) {
  return { rule, text, severity, line: line || 1, column: column || 1, end_line: endLine || line || 1, kind: syntax ? 'syntax' : 'code' };
}

async function eslint(request, batch, loaded) {
  const api = require(loaded.entry);
  const flat = path.basename(batch.config).startsWith('eslint.config.');
  const major = Number(loaded.pkg.version.split('.')[0]);
  if (![8, 9, 10].includes(major) || (!flat && major >= 10)) {
    throw failure('version_mismatch', `ESLint ${loaded.pkg.version} cannot use this ${flat ? 'flat' : 'legacy'} configuration`);
  }
  let ESLint;
  if (api.loadESLint) {
    ESLint = await api.loadESLint({ useFlatConfig: flat });
  } else {
    ESLint = flat ? createRequire(loaded.entry)('eslint/use-at-your-own-risk').FlatESLint : api.ESLint;
  }
  const options = { cwd: path.dirname(batch.config), overrideConfigFile: batch.config, fix: false, cache: false, errorOnUnmatchedPattern: true };
  if (!flat) options.resolvePluginsRelativeTo = path.dirname(batch.config);
  const linter = new ESLint(options);
  const files = [];
  for (const file of batch.files) {
    if (await linter.isPathIgnored(file)) {
      files.push({ path: file, ignored: true, messages: [] });
      continue;
    }
    const results = await linter.lintText(fs.readFileSync(file, 'utf8'), { filePath: file, warnIgnored: true });
    if (results.length !== 1) throw failure('analysis_failed', `ESLint returned no result for ${file}`);
    files.push({ path: file, ignored: false, messages: results[0].messages.map((message) => {
      if (!message.line && !message.fatal) throw failure('config_invalid', message.message);
      return row(message.ruleId || 'eslint.parse_error', message.message, message.severity === 2 ? 'error' : 'warning', message.line, message.column, message.endLine, !!message.fatal);
    }) });
  }
  return files;
}

async function stylelint(request, batch, loaded) {
  const imported = await import(pathToFileURL(loaded.entry).href);
  const api = imported.default || imported;
  const major = Number(loaded.pkg.version.split('.')[0]);
  if (major < 14 || major > 17) throw failure('version_mismatch', `Unsupported Stylelint ${loaded.pkg.version}; expected 14–17`);
  const options = { cwd: path.dirname(batch.config), fix: false, cache: false };
  if (path.basename(batch.config) === 'package.json') {
    options.config = JSON.parse(fs.readFileSync(batch.config, 'utf8')).stylelint;
    options.configBasedir = path.dirname(batch.config);
  } else {
    options.configFile = batch.config;
  }
  const files = [];
  for (const file of batch.files) {
    const result = await api.lint({ ...options, code: fs.readFileSync(file, 'utf8'), codeFilename: file });
    if (result.results.length !== 1) throw failure('analysis_failed', `Stylelint returned no result for ${file}`);
    const item = result.results[0];
    if (item.invalidOptionWarnings?.length) throw failure('config_invalid', item.invalidOptionWarnings.map((item) => item.text).join('; '));
    if ((item.warnings || []).some((message) => message.text.startsWith('Unknown rule '))) throw failure('config_invalid', item.warnings.map((item) => item.text).join('; '));
    const messages = (item.warnings || []).map((message) => row(message.rule || 'stylelint.parse_error', message.text, message.severity, message.line, message.column, message.endLine, message.rule === 'CssSyntaxError'));
    for (const message of item.parseErrors || []) messages.push(row('stylelint.parse_error', message.text, 'error', message.line, message.column, message.endLine, true));
    files.push({ path: file, ignored: !!item.ignored, messages });
  }
  return files;
}

async function contiguousClones(tokenMap, store) {
  const clones = [];
  let clone = null;
  let previous = null;
  let windowIndex = 0;
  const flush = () => {
    if (clone && this.validate(clone)) clones.push(clone);
    clone = null;
  };
  for (const frame of tokenMap) {
    frame.windowIndex = windowIndex++;
    let matched;
    try {
      matched = await store.get(frame.id);
    } catch {
      flush();
      await store.set(frame.id, frame);
      previous = null;
      continue;
    }
    // Pinned JSCPD joins repeated hashes even when the reference jumps backwards or to another file.
    if (previous && (previous.sourceId !== matched.sourceId || previous.windowIndex + 1 !== matched.windowIndex)) flush();
    clone = clone ? this.constructor.enlargeClone(clone, frame, matched) : this.constructor.createClone(tokenMap.getFormat(), frame, matched);
    previous = matched;
  }
  flush();
  return clones;
}

async function duplicates(request, batch, loaded) {
  if (loaded.pkg.version !== '4.3.0') throw failure('version_mismatch', 'Expected jscpd 4.3.0');
  const from = createRequire(loaded.entry);
  const core = from('@jscpd/core');
  const tokenizer = from('@jscpd/tokenizer');
  const { SkipLocalValidator } = from('@jscpd/finder');
  const matches = createRequire(path.join(request.bundle, 'package.json'))('picomatch');
  let native = {};
  if (batch.config) {
    native = JSON.parse(fs.readFileSync(batch.config, 'utf8'));
    if (!native || typeof native !== 'object' || Array.isArray(native)) throw failure('config_invalid', 'jscpd config must be a JSON object');
    const allowed = new Set(['$schema', 'ignore', 'ignorePattern', 'format', 'formatsExts', 'ignoreCase', 'skipLocal', 'maxSize', 'maxLines', 'minLines', 'minTokens', 'mode', 'threshold', 'reporters', 'output', 'absolute', 'gitignore']);
    for (const key of Object.keys(native)) if (!allowed.has(key)) throw failure('config_invalid', `Unsupported jscpd config option: ${key}`);
    if (native.ignore && (!Array.isArray(native.ignore) || native.ignore.some((item) => typeof item !== 'string'))) throw failure('config_invalid', 'jscpd ignore must be an array of globs');
    if (native.format && (!Array.isArray(native.format) || native.format.some((item) => !tokenizer.getSupportedFormats().includes(item)))) throw failure('config_invalid', 'jscpd format must list supported formats');
    if (native.maxLines !== undefined && (!Number.isInteger(native.maxLines) || native.maxLines < 1)) throw failure('config_invalid', 'jscpd maxLines must be positive');
  }
  const options = { ...core.getDefaultOptions(), ...native, path: batch.files, minTokens: request.duplicates.min_tokens, minLines: request.duplicates.min_lines,
    mode: request.duplicates.mode, reporters: [], silent: true, noTips: true, colors: false, absolute: true, gitignore: false,
    noSymlinks: true, store: undefined, threshold: undefined, blame: false, skipLocal: native.skipLocal || false,
    formatsExts: undefined };
  options.mode = core.getModeHandler(options.mode);
  const ignored = matches(native.ignore || [], { dot: true });
  const entries = [];
  for (const file of batch.files) {
    const relative = path.relative(request.root, file).split(path.sep).join('/');
    if (ignored(relative) || (batch.config && ignored(path.relative(path.dirname(batch.config), file).split(path.sep).join('/')))) continue;
    const extension = path.extname(file).slice(1);
    const format = ['module', 'inc', 'install', 'theme', 'profile', 'engine', 'test'].includes(extension) ? 'php'
      : tokenizer.getFormatByFile(file, native.formatsExts) || tokenizer.getFormatByFile(file);
    if (!format || (native.format && !native.format.includes(format))) continue;
    const content = fs.readFileSync(file, 'utf8');
    if (content.split('\n').length > options.maxLines) continue;
    if (native.maxSize) {
      const bytes = from('bytes');
      const limit = bytes.parse(native.maxSize);
      if (!Number.isFinite(limit) || limit <= 0) throw failure('config_invalid', 'jscpd maxSize must be a positive size');
      if (Buffer.byteLength(content) > limit) continue;
    }
    entries.push({ path: file, content, detectedFormat: format });
  }
  const store = new core.MemoryStore();
  let clones;
  try {
    const validators = options.skipLocal ? [new SkipLocalValidator()] : [];
    const detector = new core.Detector(new tokenizer.Tokenizer(), store, validators, options);
    detector.algorithm.run = contiguousClones;
    clones = [];
    for (const entry of entries.reverse()) clones.push(...await detector.detect(entry.path, entry.content, entry.detectedFormat));
  } finally {
    store.close();
  }
  const files = new Map(batch.files.map((file) => [file, { path: file, ignored: false, messages: [] }]));
  for (const clone of clones) {
    for (const [part, other] of [[clone.duplicationA, clone.duplicationB], [clone.duplicationB, clone.duplicationA]]) {
      const file = path.resolve(part.sourceId);
      if (!files.has(file)) throw failure('analysis_failed', 'jscpd returned a file outside its input set');
      files.get(file).messages.push(row('jscpd.duplicate', `Duplicate of ${JSON.stringify(path.relative(request.root, other.sourceId).split(path.sep).join('/'))}:${other.start.line}–${other.end.line}`, 'warning', part.start.line, part.start.column, part.end.line));
    }
  }
  return [...files.values()];
}

async function main() {
  const request = JSON.parse(fs.readFileSync(0, 'utf8'));
  const result = { version: 1, tool: request.id, tool_version: '', files: [], error: '', code: '' };
  try {
    for (const batch of request.groups) {
      batch.config = batch.config ? path.resolve(request.root, batch.config) : '';
      batch.files = batch.files.map((file) => path.resolve(request.root, file));
      const loaded = library(request, batch.config || path.join(request.root, '.jscpd.json'));
      result.tool_version = loaded.pkg.version;
      const run = { eslint, stylelint, jscpd: duplicates }[request.id];
      result.files.push(...await run(request, batch, loaded));
    }
  } catch (error) {
    result.files = [];
    if (['MODULE_NOT_FOUND', 'ERR_MODULE_NOT_FOUND'].includes(error.code) || ['extend-config-missing', 'plugin-missing'].includes(error.messageTemplate)) {
      const importer = error.messageData?.importerName || request.groups.find((group) => group.config)?.config;
      let directory = importer && inside(request.root, importer) ? path.dirname(importer) : request.root;
      while (directory !== request.root && !fs.existsSync(path.join(directory, 'package.json'))) {
        directory = path.dirname(directory);
      }
      const relative = path.relative(request.root, directory) || '.';
      const packageFile = path.join(directory, 'package.json');
      const pkg = fs.existsSync(packageFile) ? JSON.parse(fs.readFileSync(packageFile, 'utf8')) : {};
      const manager = typeof pkg.packageManager === 'string' ? pkg.packageManager : '';
      let install = `npm ci --prefix ${JSON.stringify(relative)}`;
      if (manager.startsWith('yarn@') || fs.existsSync(path.join(directory, 'yarn.lock'))) {
        const legacy = manager.startsWith('yarn@1.');
        install = `yarn install ${legacy ? '--frozen-lockfile' : '--immutable'} in ${JSON.stringify(relative)}`;
      } else if (manager.startsWith('pnpm@') || fs.existsSync(path.join(directory, 'pnpm-lock.yaml'))) {
        install = `pnpm install --frozen-lockfile in ${JSON.stringify(relative)}`;
      }
      error.code = 'dependencies_missing';
      error.message += `; restore the project's locked dependencies with ${install}`;
    }
    result.code = ['dependencies_missing', 'version_mismatch', 'config_invalid'].includes(error.code) ? error.code : 'analysis_failed';
    result.error = error.message || String(error);
  }
  process.stdout.write(JSON.stringify(result));
}

main().catch((error) => { process.stderr.write(error.message); process.exitCode = 1; });
