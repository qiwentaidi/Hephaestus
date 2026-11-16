#!/usr/bin/env python3
"""Create a release zip archive from a single file.

Used by build-desktop-release.sh on hosts without macOS ``ditto`` (Linux CI
runners), so release archives are produced by exactly one implementation.
"""

import os
import sys
import zipfile

CHUNK_SIZE = 1 << 20


def main(argv: list[str]) -> int:
    if len(argv) != 3:
        print("usage: release_zip.py <entry-file> <archive>", file=sys.stderr)
        return 2

    entry_path, archive_path = argv[1], argv[2]
    if not os.path.isfile(entry_path):
        print(f"missing release file: {entry_path}", file=sys.stderr)
        return 1

    entry_name = os.path.basename(entry_path)
    file_stat = os.stat(entry_path)

    info = zipfile.ZipInfo(entry_name, date_time=(1980, 1, 1, 0, 0, 0))
    info.compress_type = zipfile.ZIP_DEFLATED
    # Keep the executable bit so Linux release binaries stay runnable after unzip.
    info.external_attr = (file_stat.st_mode & 0xFFFF) << 16

    with zipfile.ZipFile(archive_path, "w", allowZip64=True) as archive:
        with archive.open(info, "w", force_zip64=True) as target:
            with open(entry_path, "rb") as source:
                while True:
                    chunk = source.read(CHUNK_SIZE)
                    if not chunk:
                        break
                    target.write(chunk)

    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
