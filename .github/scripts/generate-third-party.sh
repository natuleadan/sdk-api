#!/bin/bash
# Generate ThirdPartyNotices.txt with full license text for each dependency.
# Uses google/go-licenses to detect and save license files.

set -euo pipefail

OUTPUT="${1:-ThirdPartyNotices.txt}"
TEMP_DIR=$(mktemp -d)
CSV_FILE="$TEMP_DIR/licenses.csv"
SAVE_DIR="$TEMP_DIR/saved"
trap 'rm -rf "$TEMP_DIR"' EXIT

# === Manual overrides for packages go-licenses cannot resolve ===
resolve_override() {
  local pkg=$1
  case "$pkg" in
    "github.com/DATA-DOG/go-sqlmock")
      echo "=================================================="
      echo "  github.com/DATA-DOG/go-sqlmock"
      echo "  License: BSD-3-Clause"
      echo "=================================================="
      echo ""
      cat << 'LICEOF'
The three clause BSD license (http://en.wikipedia.org/wiki/BSD_licenses)

Copyright (c) 2013-2019, DATA-DOG team
All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met:

* Redistributions of source code must retain the above copyright notice,
  this list of conditions and the following disclaimer.

* Redistributions in binary form must reproduce the above copyright notice,
  this list of conditions and the following disclaimer in the documentation
  and/or other materials provided with the distribution.

* The name DataDog.lt may not be used to endorse or promote products
  derived from this software without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
DISCLAIMED. IN NO EVENT SHALL MICHAEL BOSTOCK BE LIABLE FOR ANY DIRECT,
INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING,
BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY
OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING
NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE,
EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
LICEOF
      echo ""
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

# === End overrides ===

echo "Installing go-licenses..."
go install github.com/google/go-licenses@latest
LICENSES_BIN="$(go env GOPATH)/bin/go-licenses"

echo "Scanning dependencies (linux/amd64)..."
GOOS=linux GOARCH=amd64 "$LICENSES_BIN" csv ./... 2>/dev/null | grep -v "natuleadan" > "$CSV_FILE"
echo "Found $(wc -l < "$CSV_FILE" | tr -d ' ') dependencies"

echo "Collecting license files (linux/amd64)..."
GOOS=linux GOARCH=amd64 "$LICENSES_BIN" save ./... --save_path="$SAVE_DIR" 2>/dev/null || true

echo "Generating $OUTPUT ..."

{
  echo "Third Party Notices"
  echo ""
  echo "This project uses third-party packages under the licenses listed below."
  echo ""

  # === Bundled / derived code (not in go.mod) ===

  echo "=================================================="
  echo "  github.com/zeromicro/go-zero (bundled)"
  echo "  License: MIT"
  echo "=================================================="
  echo ""
  echo "MIT License"
  echo ""
  echo "Copyright (c) 2022 zeromicro"
  echo ""
  echo "Permission is hereby granted, free of charge, to any person obtaining a copy"
  echo "of this software and associated documentation files (the \"Software\"), to deal"
  echo "in the Software without restriction, including without limitation the rights"
  echo "to use, copy, modify, merge, publish, distribute, sublicense, and/or sell"
  echo "copies of the Software, and to permit persons to whom the Software is"
  echo "furnished to do so, subject to the following conditions:"
  echo ""
  echo "The above copyright notice and this permission notice shall be included in all"
  echo "copies or substantial portions of the Software."
  echo ""
  echo "THE SOFTWARE IS PROVIDED \"AS IS\", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR"
  echo "IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,"
  echo "FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE"
  echo "AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER"
  echo "LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,"
  echo "OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE"
  echo "SOFTWARE."
  echo ""

  while IFS=, read -r pkg url license_type; do
    [ -z "$pkg" ] && continue
    license_type="${license_type:-Unknown}"

    # Check manual override first (for packages go-licenses cannot resolve)
    if resolve_override "$pkg" > /dev/null 2>&1; then
      resolve_override "$pkg"
      echo ""
      continue
    fi

    # If still Unknown and no override, fail the build
    if [ "$license_type" = "Unknown" ]; then
      echo "❌ Unknown license for $pkg — add override to resolve_override() in scripts/generate-third-party.sh" >&2
      exit 1
    fi

    echo "=================================================="
    echo "  $pkg"
    echo "  License: $license_type"
    echo "=================================================="
    echo ""

    # Look for the license file in the saved directory
    pkg_dir="$SAVE_DIR/$pkg"
    if [ -d "$pkg_dir" ]; then
      lic_file=$(find "$pkg_dir" -maxdepth 1 -type f \( -iname "LICENSE*" -o -iname "COPYING*" \) 2>/dev/null | head -1)
      if [ -n "$lic_file" ]; then
        cat "$lic_file"
      else
        echo "(License file not found in $pkg)"
      fi
    else
      # Some CSV entries are sub-packages; try parent module
      parent="$pkg_dir"
      while [ "$parent" != "$SAVE_DIR" ]; do
        parent=$(dirname "$parent")
        if [ -d "$parent" ]; then
          lic_file=$(find "$parent" -maxdepth 1 -type f \( -iname "LICENSE*" -o -iname "COPYING*" \) 2>/dev/null | head -1)
          if [ -n "$lic_file" ]; then
            cat "$lic_file"
            break
          fi
        fi
      done
      if [ -z "${lic_file:-}" ]; then
        echo "(License file not found in $pkg)"
      fi
    fi
    echo ""
  done < "$CSV_FILE"
} > "$OUTPUT"

echo "Done — generated $(wc -l < "$OUTPUT" | tr -d ' ') lines"
