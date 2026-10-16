Unicode true

# Defines read by wails_tools.nsh. The install dir, uninstall key and shortcuts use the
# product name alone, and the app is installed under the name the zip ships it as.
# The in-app updater relies on all of these, so change them together with it.
!define PRODUCT_EXECUTABLE "manga-downloader-gui.exe"
!define UNINST_KEY_NAME    "Manga Downloader"

!include "wails_tools.nsh"

!insertmacro GetParameters
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
!define MUI_PAGE_CUSTOMFUNCTION_SHOW FinishShow

!insertmacro MUI_PAGE_WELCOME # Welcome to the installer page.
# !insertmacro MUI_PAGE_LICENSE "resources\eula.txt" # Adds a EULA page to the installer
!insertmacro MUI_PAGE_INSTFILES # Installing page.
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

Function .onInit
    !insertmacro wails.checkArchitecture

    SetRegView 64
    ReadRegStr $0 HKLM "${UNINST_KEY}" "InstallLocation"
    ${If} $0 != ""
        StrCpy $INSTDIR $0
    ${EndIf}

    # A running exe can't be opened for writing, which tells us the app is still open.
    StrCpy $2 0
    check:
    IfFileExists "$INSTDIR\${PRODUCT_EXECUTABLE}" 0 done
    ClearErrors
    FileOpen $1 "$INSTDIR\${PRODUCT_EXECUTABLE}" a
    IfErrors busy
    FileClose $1
    Goto done
    busy:
    IfSilent +3
    MessageBox MB_RETRYCANCEL|MB_ICONEXCLAMATION "${INFO_PRODUCTNAME} is running. Close it, then click Retry." IDRETRY check
    Abort
    IntOp $2 $2 + 1
    ${If} $2 > 10
        Abort
    ${EndIf}
    Sleep 1000
    Goto check
    done:
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
    IfErrors +4
    # 1203 is the Run checkbox's control id on the finish page.
    FindWindow $0 "#32770" "" $HWNDPARENT
    GetDlgItem $0 $0 1203
    ShowWindow $0 ${SW_HIDE}
FunctionEnd

Section
    !insertmacro wails.setShellContext

    !insertmacro wails.webview2runtime

    SetOutPath $INSTDIR

    !insertmacro wails.files

    CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"

    !insertmacro wails.associateFiles
    !insertmacro wails.associateCustomProtocols

    !insertmacro wails.writeUninstaller

    # The in-app updater and the next install read the install dir from here.
    SetRegView 64
    WriteRegStr HKLM "${UNINST_KEY}" "InstallLocation" "$INSTDIR"
SectionEnd

Section "uninstall"
    !insertmacro wails.setShellContext

    # The WebView2 data path is per user, whereas the shortcut below is per machine.
    SetShellVarContext current
    RMDir /r "$AppData\${PRODUCT_EXECUTABLE}"
    SetShellVarContext all

    Delete "$INSTDIR\${PRODUCT_EXECUTABLE}"

    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"

    !insertmacro wails.unassociateFiles
    !insertmacro wails.unassociateCustomProtocols

    !insertmacro wails.deleteUninstaller

    # Not recursive: a folder with anything else in it is left alone.
    RMDir $INSTDIR
SectionEnd
