Unicode true

# Defines read by wails_tools.nsh. Install dir, uninstall key and shortcuts use the product
# name alone, the exe keeps the zip's name, and the scope (elevated: per machine in HKLM,
# else per user in HKCU) is picked at run time. The in-app updater relies on all of this.
!define REQUEST_EXECUTION_LEVEL "user"
!define PRODUCT_EXECUTABLE "manga-downloader-gui.exe"
!define UNINST_KEY_NAME    "Manga Downloader"

!include "wails_tools.nsh"

!include "WordFunc.nsh"

!insertmacro GetParameters
!insertmacro VersionCompare
!insertmacro un.GetParameters
!insertmacro un.GetOptions
!insertmacro GetSize

Var MachineScope
Var ExistingInstall
!insertmacro GetOptions

# The version information for this two must consist of 4 parts
VIProductVersion "${INFO_PRODUCTVERSION}.0"
VIFileVersion    "${INFO_PRODUCTVERSION}.0"

VIAddVersionKey "CompanyName"     "${INFO_COMPANYNAME}"
VIAddVersionKey "FileDescription" "${INFO_PRODUCTNAME} Installer"
VIAddVersionKey "ProductVersion"  "${INFO_PRODUCTVERSION}"
VIAddVersionKey "FileVersion"     "${INFO_PRODUCTVERSION}"
VIAddVersionKey "LegalCopyright"  "${INFO_COPYRIGHT}"
VIAddVersionKey "ProductName"     "${INFO_PRODUCTNAME}"

# Enable HiDPI support. https://nsis.sourceforge.io/Reference/ManifestDPIAware
ManifestDPIAware true

!include "MUI.nsh"

!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
# !define MUI_WELCOMEFINISHPAGE_BITMAP "resources\leftimage.bmp" #Include this to add a bitmap on the left side of the Welcome Page. Must be a size of 164x314
!define MUI_FINISHPAGE_NOAUTOCLOSE # Wait on the INSTFILES page so the user can take a look into the details of the installation steps
!define MUI_ABORTWARNING # This will warn the user if they exit from the installer.
!define MUI_FINISHPAGE_RUN
!define MUI_FINISHPAGE_RUN_FUNCTION FinishRun

!insertmacro MUI_PAGE_WELCOME # Welcome to the installer page.
# !insertmacro MUI_PAGE_LICENSE "resources\eula.txt" # Adds a EULA page to the installer
!define MUI_PAGE_CUSTOMFUNCTION_PRE DirPre
!define MUI_PAGE_CUSTOMFUNCTION_LEAVE DirLeave
!insertmacro MUI_PAGE_DIRECTORY # In which folder install page.
!insertmacro MUI_PAGE_INSTFILES # Installing page.
!define MUI_PAGE_CUSTOMFUNCTION_SHOW FinishShow
!insertmacro MUI_PAGE_FINISH # Finished installation page.

!insertmacro MUI_UNPAGE_INSTFILES # Uinstalling page

!insertmacro MUI_LANGUAGE "English" # Set the Language of the installer

## The following two statements can be used to sign the installer and the uninstaller. The path to the binaries are provided in %1
#!uninstfinalize 'signtool --file "%1"'
#!finalize 'signtool --file "%1"'

Name "${INFO_PRODUCTNAME}"
OutFile "..\..\bin\manga-downloader-gui-${ARCH}-installer.exe" # Name of the installer's file.
InstallDir "$PROGRAMFILES64\${INFO_PRODUCTNAME}" # Never under the company name.
ShowInstDetails show # This will always show the installation details.

# Whether this process is elevated: only an admin token can write under HKLM\Software.
!macro IsElevated RESULT
    SetRegView 64
    ClearErrors
    WriteRegStr HKLM "Software\${INFO_PRODUCTNAME}Probe" "" ""
    ${If} ${Errors}
        StrCpy ${RESULT} 0
    ${Else}
        DeleteRegKey HKLM "Software\${INFO_PRODUCTNAME}Probe"
        StrCpy ${RESULT} 1
    ${EndIf}
!macroend

Function .onInit
    !insertmacro wails.checkArchitecture

    SetRegView 64
    !insertmacro IsElevated $3
    ${If} $3 == 1
        SetShellVarContext all
        StrCpy $INSTDIR "$PROGRAMFILES64\${INFO_PRODUCTNAME}"
        # A per-user copy stays: it has its own uninstall entry.
        ReadRegStr $0 HKCU "${UNINST_KEY}" "InstallLocation"
        ${If} $0 != ""
            IfSilent +2
            MessageBox MB_OK|MB_ICONINFORMATION "A per-user copy of ${INFO_PRODUCTNAME} is installed in $0. It stays; remove it from Add or remove programs if you no longer need it."
        ${EndIf}
    ${Else}
        SetShellVarContext current
        StrCpy $INSTDIR "$LOCALAPPDATA\Programs\${INFO_PRODUCTNAME}"
        # Only a per-machine install exists: upgrade it in place, which needs elevation.
        # /elevated marks the relaunch so a failed elevation can't loop.
        ReadRegStr $0 HKCU "${UNINST_KEY}" "InstallLocation"
        ReadRegStr $2 HKLM "${UNINST_KEY}" "InstallLocation"
        ${If} $0 == ""
        ${AndIf} $2 != ""
            ${GetParameters} $1
            ${GetOptions} $1 "/elevated" $3
            ${IfNot} ${Errors}
                MessageBox MB_OK|MB_ICONSTOP "${INFO_PRODUCTNAME} is installed for all users, but administrator rights couldn't be obtained."
                Abort
            ${EndIf}
            ClearErrors
            ExecShell "runas" "$EXEPATH" "/elevated $1"
            IfErrors 0 +3
            MessageBox MB_OK|MB_ICONSTOP "${INFO_PRODUCTNAME} is installed for all users, so updating it needs administrator rights."
            Abort
            Quit
        ${EndIf}
    ${EndIf}

    StrCpy $MachineScope $3
    StrCpy $ExistingInstall 0
    ReadRegStr $0 SHCTX "${UNINST_KEY}" "InstallLocation"
    ${If} $0 != ""
        StrCpy $INSTDIR $0
        StrCpy $ExistingInstall 1
    ${EndIf}

    # Downgrading asks first, and never happens silently.
    ReadRegStr $0 SHCTX "${UNINST_KEY}" "DisplayVersion"
    ${If} $0 != ""
        ${VersionCompare} "$0" "${INFO_PRODUCTVERSION}" $1
        ${If} $1 == 1
            IfSilent 0 +2
            Abort
            MessageBox MB_YESNO|MB_ICONEXCLAMATION "${INFO_PRODUCTNAME} $0 is already installed, which is newer than this installer (${INFO_PRODUCTVERSION}). Install the older version anyway?" IDYES +2
            Abort
        ${EndIf}
    ${EndIf}

    # A running exe can't be opened for writing, which tells us the app is still open.
    # The in-app updater starts us with /relaunch and quits right after, so then wait
    # for it before asking; a silent install only waits.
    ${GetParameters} $3
    ClearErrors
    ${GetOptions} $3 "/relaunch" $3
    ${If} ${Errors}
        StrCpy $3 0
    ${Else}
        StrCpy $3 1
    ${EndIf}
    StrCpy $2 0
    check:
    IfFileExists "$INSTDIR\${PRODUCT_EXECUTABLE}" 0 done
    ClearErrors
    FileOpen $1 "$INSTDIR\${PRODUCT_EXECUTABLE}" a
    IfErrors busy
    FileClose $1
    Goto done
    busy:
    IntOp $2 $2 + 1
    ${If} $2 <= 10
        ${If} ${Silent}
        ${OrIf} $3 == 1
            Sleep 1000
            Goto check
        ${EndIf}
    ${EndIf}
    IfSilent 0 +2
    Abort
    MessageBox MB_RETRYCANCEL|MB_ICONEXCLAMATION "${INFO_PRODUCTNAME} is running. Close it, then click Retry." IDRETRY check
    Abort
    done:
FunctionEnd

# An upgrade keeps the recorded folder, so it skips the page. /D= and silent installs have no
# page either, and use the scope default or the existing location by design.
Function DirPre
    ${If} $ExistingInstall == 1
        Abort
    ${EndIf}
FunctionEnd

Function DirLeave
    StrCpy $0 $INSTDIR 1 -1
    ${If} $0 == "\"
        StrCpy $INSTDIR $INSTDIR -1
    ${EndIf}
    # Only X:\... or UNC paths; a drive root, a share root or the bare parent folders get our own subfolder.
    StrLen $0 $INSTDIR
    StrCpy $2 $INSTDIR 2
    ${If} $2 == "\\"
        StrCpy $3 0
        StrCpy $5 0
        countloop:
        StrCmp $5 $0 countdone
        StrCpy $6 $INSTDIR 1 $5
        ${If} $6 == "\"
            IntOp $3 $3 + 1
        ${EndIf}
        IntOp $5 $5 + 1
        Goto countloop
        countdone:
        ${If} $3 < 3
            Goto invalid
        ${EndIf}
        ${If} $3 == 3
            StrCpy $INSTDIR "$INSTDIR\${INFO_PRODUCTNAME}"
        ${EndIf}
    ${Else}
        StrCpy $6 $INSTDIR 1 1
        ${If} $6 != ":"
            Goto invalid
        ${EndIf}
        ${If} $0 == 2
            StrCpy $INSTDIR "$INSTDIR\${INFO_PRODUCTNAME}"
        ${ElseIf} $0 > 2
            StrCpy $6 $INSTDIR 1 2
            ${If} $6 != "\"
                Goto invalid
            ${EndIf}
        ${EndIf}
    ${EndIf}
    ${If} $INSTDIR == "$PROGRAMFILES64"
    ${OrIf} $INSTDIR == "$LOCALAPPDATA\Programs"
        StrCpy $INSTDIR "$INSTDIR\${INFO_PRODUCTNAME}"
    ${EndIf}

    ${If} $MachineScope != 1
        # Not elevated: the folder must be writable. Only the last folder is ever created,
        # so a failed probe leaves nothing behind once it's removed.
        StrCpy $1 0
        IfFileExists "$INSTDIR\*.*" probe
        StrLen $4 $INSTDIR
        StrCpy $5 $4
        parentloop:
        IntOp $5 $5 - 1
        StrCpy $6 $INSTDIR 1 $5
        StrCmp $6 "\" parentfound
        StrCmp $5 0 failed
        Goto parentloop
        parentfound:
        StrCpy $7 $INSTDIR $5
        IfFileExists "$7\*.*" 0 failed
        StrCpy $1 1
        CreateDirectory "$INSTDIR"
        probe:
        ClearErrors
        FileOpen $0 "$INSTDIR\.write-probe" w
        IfErrors failed
        FileClose $0
        Delete "$INSTDIR\.write-probe"
        Return
        failed:
        ${If} $1 == 1
            RMDir "$INSTDIR"
        ${EndIf}
        MessageBox MB_OK|MB_ICONEXCLAMATION "You can't install there without administrator rights. Pick another folder or run the installer as administrator."
        Abort
    ${EndIf}
    Return

    invalid:
    MessageBox MB_OK|MB_ICONEXCLAMATION "Pick a folder like C:\Apps\${INFO_PRODUCTNAME}, or a network path."
    Abort
FunctionEnd

# Through explorer.exe so the app doesn't inherit the installer's elevation.
Function LaunchApp
    Exec '"$WINDIR\explorer.exe" "$INSTDIR\${PRODUCT_EXECUTABLE}"'
FunctionEnd

# The in-app updater passes /relaunch to start the new version once installed.
Function .onInstSuccess
    ${GetParameters} $0
    ${GetOptions} $0 "/relaunch" $1
    IfErrors +2
    Call LaunchApp
FunctionEnd

# With /relaunch the app is already started, so the finish page must not start it again.
Function FinishRun
    ${GetParameters} $0
    ${GetOptions} $0 "/relaunch" $1
    IfErrors 0 +2
    Call LaunchApp
FunctionEnd

Function FinishShow
    ${GetParameters} $0
    ${GetOptions} $0 "/relaunch" $1
    IfErrors +5
    # 1203 is the Run checkbox's control id on the finish page.
    FindWindow $0 "#32770" "" $HWNDPARENT
    GetDlgItem $0 $0 1203
    ShowWindow $0 ${SW_HIDE}
    SendMessage $0 ${BM_SETCHECK} ${BST_UNCHECKED} 0
FunctionEnd

Section
    !insertmacro wails.webview2runtime

    SetOutPath $INSTDIR

    !insertmacro wails.files

    CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"

    !insertmacro wails.associateFiles
    !insertmacro wails.associateCustomProtocols

    WriteUninstaller "$INSTDIR\uninstall.exe"

    # SHCTX follows SetShellVarContext: HKLM for a per-machine install, HKCU for a per-user one.
    # The in-app updater reads InstallLocation and DisplayVersion from here.
    SetRegView 64
    WriteRegStr SHCTX "${UNINST_KEY}" "Publisher" "${INFO_COMPANYNAME}"
    WriteRegStr SHCTX "${UNINST_KEY}" "DisplayName" "${INFO_PRODUCTNAME}"
    WriteRegStr SHCTX "${UNINST_KEY}" "DisplayVersion" "${INFO_PRODUCTVERSION}"
    WriteRegStr SHCTX "${UNINST_KEY}" "DisplayIcon" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    WriteRegStr SHCTX "${UNINST_KEY}" "InstallLocation" "$INSTDIR"
    WriteRegStr SHCTX "${UNINST_KEY}" "UninstallString" "$\"$INSTDIR\uninstall.exe$\""
    WriteRegStr SHCTX "${UNINST_KEY}" "QuietUninstallString" "$\"$INSTDIR\uninstall.exe$\" /S"
    ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
    IntFmt $0 "0x%08X" $0
    WriteRegDWORD SHCTX "${UNINST_KEY}" "EstimatedSize" "$0"
SectionEnd

# The scope follows where uninstall.exe lives. A per-machine uninstall needs admin, which
# Apps & features doesn't grant, so it relaunches itself elevated.
Function un.onInit
    SetRegView 64
    # The scope is whichever install is recorded at this folder; the folder page can put either anywhere.
    ReadRegStr $0 HKCU "${UNINST_KEY}" "InstallLocation"
    StrCpy $1 $0 1 -1
    ${If} $1 == "\"
        StrCpy $0 $0 -1
    ${EndIf}
    ${If} $0 != ""
    ${AndIf} $0 == $INSTDIR
        SetShellVarContext current
        StrCpy $MachineScope 0
    ${Else}
        SetShellVarContext all
        StrCpy $MachineScope 1
        ClearErrors
        WriteRegStr HKLM "Software\${INFO_PRODUCTNAME}Probe" "" ""
        ${If} ${Errors}
            ${un.GetParameters} $1
            ${un.GetOptions} $1 "/elevated" $2
            ${IfNot} ${Errors}
                MessageBox MB_OK|MB_ICONSTOP "Uninstalling ${INFO_PRODUCTNAME} for all users needs administrator rights, which couldn't be obtained."
                Abort
            ${EndIf}
            # Not $EXEPATH, which is NSIS's temp copy of the uninstaller.
            StrCpy $1 "/elevated"
            IfSilent 0 +2
            StrCpy $1 "/S /elevated"
            ClearErrors
            ExecShell "runas" "$INSTDIR\uninstall.exe" "$1"
            IfErrors 0 +3
            MessageBox MB_OK|MB_ICONSTOP "Uninstalling ${INFO_PRODUCTNAME} for all users needs administrator rights."
            Abort
            Quit
        ${EndIf}
        DeleteRegKey HKLM "Software\${INFO_PRODUCTNAME}Probe"
    ${EndIf}
FunctionEnd

Section "uninstall"
    # The WebView2 data path is per user, whereas the shortcut below follows the scope.
    SetShellVarContext current
    RMDir /r "$AppData\${PRODUCT_EXECUTABLE}"
    ${If} $MachineScope == 1
        SetShellVarContext all
    ${EndIf}

    Delete "$INSTDIR\${PRODUCT_EXECUTABLE}"

    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"

    !insertmacro wails.unassociateFiles
    !insertmacro wails.unassociateCustomProtocols

    Delete "$INSTDIR\uninstall.exe"
    SetRegView 64
    DeleteRegKey SHCTX "${UNINST_KEY}"

    # Not recursive: a folder with anything else in it is left alone.
    RMDir $INSTDIR
SectionEnd
