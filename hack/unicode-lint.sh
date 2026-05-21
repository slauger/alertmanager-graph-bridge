#!/usr/bin/env bash
#
# unicode-lint.sh detects suspicious Unicode characters that commonly slip in
# from copy-pasted or AI-generated text: smart quotes, invisible characters,
# non-breaking spaces, decorative dashes and box-drawing characters. It exits
# non-zero when any are found.
set -euo pipefail
export LC_ALL=C.UTF-8

# Disallowed code points (PCRE \x{...} escapes):
#   00A0            non-breaking space
#   2000-200F       en/em spaces, zero-width spaces, joiners, marks
#   2010-2015       hyphens, en/em dashes
#   2018 2019       single curly quotes
#   201C 201D       double curly quotes
#   2022 2026       bullet, horizontal ellipsis
#   2028 2029       line/paragraph separators
#   202F 2060       narrow no-break space, word joiner
#   2039 203A       single guillemets
#   2212            minus sign
#   2500-257F       box-drawing characters
#   FEFF            byte order mark
pattern='[\x{00A0}\x{2000}-\x{200F}\x{2010}-\x{2015}\x{2018}\x{2019}\x{201C}\x{201D}\x{2022}\x{2026}\x{2028}\x{2029}\x{202F}\x{2039}\x{203A}\x{2060}\x{2212}\x{2500}-\x{257F}\x{FEFF}]'

status=0
while IFS= read -r file; do
  [ -n "$file" ] || continue
  [ -f "$file" ] || continue
  if matches=$(grep -nP "$pattern" "$file" 2>/dev/null); then
    echo "Suspicious Unicode characters in ${file}:"
    echo "${matches}"
    status=1
  fi
done < <(git ls-files -- '*.go' '*.md' '*.yaml' '*.yml' '*.json' '*.tpl' '*.html' '*.sh' 'Makefile' '*Containerfile')

if [ "$status" -eq 0 ]; then
  echo "unicode-lint: no suspicious characters found."
fi
exit "$status"
