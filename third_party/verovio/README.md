# Verovio

score2audio uses [Verovio](https://www.verovio.org/) (by the RISM Digital Center and contributors) to engrave the music shown in videos. Verovio is free software under the GNU Lesser General Public License, version 3 or later: see COPYING.LESSER and COPYING here.

It is not built into the score2audio program. The downloads include an unmodified copy of `verovio-toolkit-wasm.js` from the npm package `verovio` version 6.4.1 (checked against its published SHA-512 fingerprint when the downloads are built), in a folder called `verovio` next to the program. The program loads it from there, so you can replace it with another build of Verovio, including one you have built yourself from its source code at https://github.com/rism-digital/verovio.
