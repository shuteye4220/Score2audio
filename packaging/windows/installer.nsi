; The score2audio installer for Windows. Built by "make dist" with NSIS:
;   makensis -DVERSION=1.1.0 -DSRC=build/windows -DOUTFILE=dist/score2audio-windows-setup.exe packaging/windows/installer.nsi
;
; It installs for the current user only (no administrator rights needed),
; adds score2audio to the Start menu and to Apps in Settings, and then runs
; "score2audio --setup --yes", which downloads Audiveris and FFmpeg from their
; publishers, checks each download against its known SHA-256, and installs
; them. Windows asks before the Audiveris installer makes changes.
;
; Run with /S /UPDATE (by score2audio itself, for an automatic update), it
; waits for score2audio to close, replaces it without asking anything, and
; starts it again.

Unicode true
!include "MUI2.nsh"
!include "FileFunc.nsh"
!include "LogicLib.nsh"

!ifndef VERSION
  !define VERSION "dev"
!endif
!ifndef SRC
  !define SRC "build/windows"
!endif
!ifndef OUTFILE
  !define OUTFILE "score2audio-windows-setup.exe"
!endif
!define UNINSTKEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\score2audio"

Name "score2audio"
Caption "score2audio ${VERSION} setup"
OutFile "${OUTFILE}"
RequestExecutionLevel user
InstallDir "$LOCALAPPDATA\Programs\score2audio"
InstallDirRegKey HKCU "Software\score2audio" "InstallDir"
SetCompressor /SOLID lzma
ShowInstDetails show
ShowUninstDetails show
BrandingText "score2audio ${VERSION}"

Var IsUpdate

!define MUI_ABORTWARNING
!define MUI_WELCOMEPAGE_TITLE "Install score2audio ${VERSION}"
!define MUI_WELCOMEPAGE_TEXT "score2audio reads sheet music from PDF files and plays the parts you choose, with the lyrics.$\r$\n$\r$\nThis installs score2audio for you on this computer. It can also download and install the free programs it uses: Audiveris, which reads the music in PDF files, and FFmpeg, for MP3 files and video sound. They come from their own publishers, and each download is checked before it is used.$\r$\n$\r$\nChoose Next to continue."
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_LICENSE "${SRC}/LICENSE.txt"
!define MUI_COMPONENTSPAGE_SMALLDESC
!insertmacro MUI_PAGE_COMPONENTS
!insertmacro MUI_PAGE_INSTFILES
!define MUI_FINISHPAGE_TITLE "score2audio is installed"
!define MUI_FINISHPAGE_TEXT "Start score2audio from the Start menu. It opens in your web browser.$\r$\n$\r$\nThe details of the installation, including the helper programs, are listed on the previous page."
!define MUI_FINISHPAGE_RUN "$INSTDIR\score2audio.exe"
!define MUI_FINISHPAGE_RUN_TEXT "Start score2audio now"
!insertmacro MUI_PAGE_FINISH

!define MUI_UNCONFIRMPAGE_TEXT_TOP "This removes score2audio from this computer. Your scores and settings (in $LOCALAPPDATA\score2audio) are kept, and so are Audiveris and FFmpeg if they were installed separately."
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"

Function .onInit
  StrCpy $IsUpdate 0
  ${GetParameters} $R0
  ClearErrors
  ${GetOptions} $R0 "/UPDATE" $R1
  ${IfNot} ${Errors}
    StrCpy $IsUpdate 1
  ${EndIf}
FunctionEnd

Section "score2audio (required)" SecMain
  SectionIn RO
  SetOutPath "$INSTDIR"
  ${If} $IsUpdate = 1
    ; wait (up to 30 seconds) for the old score2audio to close
    StrCpy $0 0
    ${Do}
      ClearErrors
      FileOpen $1 "$INSTDIR\score2audio.exe" a
      ${IfNot} ${Errors}
        FileClose $1
        ${Break}
      ${EndIf}
      IntOp $0 $0 + 1
      ${If} $0 > 60
        ${Break}
      ${EndIf}
      Sleep 500
    ${Loop}
  ${EndIf}
  File "${SRC}/score2audio.exe"
  File "${SRC}/README.txt"
  File "${SRC}/LICENSE.txt"
  SetOutPath "$INSTDIR\tessdata"
  File /r "${SRC}/tessdata/*"
  SetOutPath "$INSTDIR\verovio"
  File /r "${SRC}/verovio/*"
  SetOutPath "$INSTDIR"

  WriteUninstaller "$INSTDIR\uninstall.exe"
  WriteRegStr HKCU "Software\score2audio" "InstallDir" "$INSTDIR"
  WriteRegStr HKCU "${UNINSTKEY}" "DisplayName" "score2audio"
  WriteRegStr HKCU "${UNINSTKEY}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "${UNINSTKEY}" "Publisher" "score2audio"
  WriteRegStr HKCU "${UNINSTKEY}" "URLInfoAbout" "https://github.com/shuteye4220/Score2audio"
  WriteRegStr HKCU "${UNINSTKEY}" "DisplayIcon" "$INSTDIR\score2audio.exe"
  WriteRegStr HKCU "${UNINSTKEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "${UNINSTKEY}" "UninstallString" '"$INSTDIR\uninstall.exe"'
  WriteRegStr HKCU "${UNINSTKEY}" "QuietUninstallString" '"$INSTDIR\uninstall.exe" /S'
  WriteRegDWORD HKCU "${UNINSTKEY}" "NoModify" 1
  WriteRegDWORD HKCU "${UNINSTKEY}" "NoRepair" 1
  ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
  WriteRegDWORD HKCU "${UNINSTKEY}" "EstimatedSize" $0

  CreateShortCut "$SMPROGRAMS\score2audio.lnk" "$INSTDIR\score2audio.exe" "" "$INSTDIR\score2audio.exe" 0 SW_SHOWNORMAL "" "Read sheet music PDFs and play the parts"

  ${If} $IsUpdate = 1
    ; start the new version, as the old one was running
    Exec '"$INSTDIR\score2audio.exe"'
  ${EndIf}
SectionEnd

Section "Audiveris and FFmpeg (downloads about 200 MB)" SecHelpers
  ${If} $IsUpdate = 1
    Return
  ${EndIf}
  DetailPrint "Setting up the helper programs. The downloads can take several minutes."
  nsExec::ExecToLog '"$INSTDIR\score2audio.exe" --setup --yes'
  Pop $0
  ${If} $0 != 0
    DetailPrint "Audiveris could not be installed now. score2audio can still open MusicXML files, and its Setup section can try again or explain how to install Audiveris yourself."
  ${EndIf}
SectionEnd

Section /o "Shortcut on the desktop" SecDesktop
  CreateShortCut "$DESKTOP\score2audio.lnk" "$INSTDIR\score2audio.exe"
SectionEnd

!insertmacro MUI_FUNCTION_DESCRIPTION_BEGIN
  !insertmacro MUI_DESCRIPTION_TEXT ${SecMain} "score2audio itself, with the data it needs to read lyrics and make videos."
  !insertmacro MUI_DESCRIPTION_TEXT ${SecHelpers} "Downloads and installs Audiveris (needed to read PDF files) and FFmpeg (optional), if they are not installed yet."
  !insertmacro MUI_DESCRIPTION_TEXT ${SecDesktop} "Adds a score2audio shortcut to the desktop as well as the Start menu."
!insertmacro MUI_FUNCTION_DESCRIPTION_END

Section "Uninstall"
  Delete "$INSTDIR\score2audio.exe"
  Delete "$INSTDIR\score2audio.exe.old"
  Delete "$INSTDIR\README.txt"
  Delete "$INSTDIR\LICENSE.txt"
  RMDir /r "$INSTDIR\tessdata"
  RMDir /r "$INSTDIR\verovio"
  RMDir /r "$INSTDIR\ffmpeg"
  Delete "$INSTDIR\uninstall.exe"
  RMDir "$INSTDIR"
  Delete "$SMPROGRAMS\score2audio.lnk"
  Delete "$DESKTOP\score2audio.lnk"
  DeleteRegKey HKCU "${UNINSTKEY}"
  DeleteRegKey HKCU "Software\score2audio"
SectionEnd
