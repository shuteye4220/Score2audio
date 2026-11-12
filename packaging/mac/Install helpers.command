#!/bin/bash
# Double-click this file to install the programs score2audio uses: Audiveris,
# which reads the music in PDF files, and FFmpeg (optional, for MP3 files and
# video sound; needs Homebrew). Each download is checked before it is used.
cd "$(dirname "$0")" || exit 1
xattr -dr com.apple.quarantine . 2>/dev/null
./score2audio --setup
echo
read -r -p "Press Return to close this window. " _
