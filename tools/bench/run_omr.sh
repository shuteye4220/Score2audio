#!/bin/bash
# Reads every case's PDF through score2audio's own reading path (--read).
# usage: run_omr.sh CASESDIR SCORE2AUDIO-BINARY
# Set AUDIVERIS_EXE to Audiveris's launcher and TESSDATA_PREFIX to a tessdata
# folder whose eng.traineddata includes the legacy engine.
set -u
cases=$1 bin=$2
for d in "$cases"/*/; do
  [ -n "$(ls "$d"omr/*.mxl 2>/dev/null)" ] && continue
  rm -rf "$d/omr"
  start=$(date +%s)
  timeout 1200 "$bin" --read "$d/score.pdf" "$d/omr" > "$d/read.txt" 2>&1
  echo "$(basename "$d") $(( $(date +%s) - start ))s $(ls "$d"omr/*.mxl 2>/dev/null | wc -l) mxl $(grep -h 'pages without' "$d/read.txt")"
done
