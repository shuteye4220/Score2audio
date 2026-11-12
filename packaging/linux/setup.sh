#!/bin/sh
# Installs the programs score2audio uses: Audiveris, which reads the music in
# PDF files, and FFmpeg (optional, for MP3 files and video sound). It uses
# your distribution's packages where it has them, and asks before running
# anything that needs your password.
cd "$(dirname "$0")" || exit 1
exec ./score2audio --setup
