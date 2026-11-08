import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname } from 'node:path';

const source = readFileSync(new URL('../../../pkg/version/version.go', import.meta.url), 'utf8');
const [, version] = /^const DesktopVersion = "v?(\d+\.\d+\.\d+)"\r?$/m.exec(source) ?? [];
if (!version || version.split('.').some((part) => Number(part) > 65535)) {
  throw new Error('DesktopVersion must have three numeric components between 0 and 65535');
}

const [format, input, output] = process.argv.slice(2);

function writeMetadata(contents) {
  mkdirSync(dirname(output), { recursive: true });
  writeFileSync(output, contents);
}

switch (format) {
  case 'version':
    console.log(version);
    break;
  case 'windows': {
    const metadata = JSON.parse(readFileSync(input, 'utf8'));
    metadata.fixed.file_version = version;
    metadata.fixed.product_version = version;
    for (const strings of Object.values(metadata.info)) {
      strings.FileVersion = version;
      strings.ProductVersion = version;
    }
    writeMetadata(JSON.stringify(metadata, null, 2) + '\n');
    break;
  }
  case 'plist': {
    let metadata = readFileSync(input, 'utf8');
    for (const key of ['CFBundleShortVersionString', 'CFBundleVersion']) {
      const field = new RegExp('(<key>' + key + '</key>\\s*<string>)[^<]*(</string>)');
      if (!field.test(metadata)) {
        throw new Error(input + ' is missing ' + key);
      }
      metadata = metadata.replace(field, (_match, start, end) => start + version + end);
    }
    writeMetadata(metadata);
    break;
  }
  default:
    throw new Error('Usage: desktop-version.mjs version | (windows|plist) <input> <output>');
}
