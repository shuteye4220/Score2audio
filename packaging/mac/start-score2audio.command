#!/bin/bash
# Double-click this file to start score2audio. A Terminal window opens and
# shows its messages; close that window (or use Quit on the page) to stop it.
cd "$(dirname "$0")" || exit 1
# Files downloaded from the internet are blocked until approved; this clears
# that mark for score2audio's own folder only.
xattr -dr com.apple.quarantine . 2>/dev/null
exec ./score2audio
