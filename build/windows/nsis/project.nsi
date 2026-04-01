Unicode true

####
## Please note: Template replacements don't work in this file. They are provided with default defines like
## mentioned underneath.
## If the keyword is not defined, "wails_tools.nsh" will populate them.
## If they are defined here, "wails_tools.nsh" will not touch them. This allows you to use this project.nsi manually
## from outside of Wails for debugging and development of the installer.
## 
## For development first make a wails nsis build to populate the "wails_tools.nsh":
## > wails build --target windows/amd64 --nsis
## Then you can call makensis on this file with specifying the path to your binary:
## For a AMD64 only installer:
## > makensis -DARG_WAILS_AMD64_BINARY=..\..\bin\app.exe
## For a ARM64 only installer:
## > makensis -DARG_WAILS_ARM64_BINARY=..\..\bin\app.exe
## For a installer with both architectures:
## > makensis -DARG_WAILS_AMD64_BINARY=..\..\bin\app-amd64.exe -DARG_WAILS_ARM64_BINARY=..\..\bin\app-arm64.exe
####
## The following information is taken from the wails_tools.nsh file, but they can be overwritten here.
####
## !define INFO_PROJECTNAME    "my-project" # Default "ChatClaw"
## !define INFO_COMPANYNAME    "My Company" # Default "My Company"
## !define INFO_PRODUCTNAME    "My Product Name" # Default "My Product"
## !define INFO_PRODUCTVERSION "1.0.0"     # Default "0.1.0"
## !define INFO_COPYRIGHT      "(c) Now, My Company" # Default "© 2026, My Company"
###
## !define PRODUCT_EXECUTABLE  "Application.exe"      # Default "${INFO_PROJECTNAME}.exe"
## !define UNINST_KEY_NAME     "UninstKeyInRegistry"  # Default "${INFO_COMPANYNAME}${INFO_PRODUCTNAME}"
####
## Install to per-user directory so auto-update can overwrite the binary without
## needing admin/UAC elevation. This follows the pattern used by VS Code, Discord, etc.
!define REQUEST_EXECUTION_LEVEL "user"
####
## Include the wails tools
####
!include "wails_tools.nsh"

; When -DBUNDLE_OPENCLAW=1 is passed by makensis, !ifdef BUNDLE_OPENCLAW evaluates true.
!ifdef BUNDLE_OPENCLAW
!endif

# Per-user install: register URL scheme under HKCU so browser can launch the app without admin.
# SHELL_CONTEXT is used by wails macros and our chatclaw registration; must match REQUEST_EXECUTION_LEVEL.
!define SHELL_CONTEXT HKCU

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

# Launch the application after installation
!define MUI_FINISHPAGE_RUN "$INSTDIR\${PRODUCT_EXECUTABLE}"
!define MUI_FINISHPAGE_RUN_TEXT "Run ${INFO_PRODUCTNAME}"

!insertmacro MUI_PAGE_WELCOME # Welcome to the installer page.
# !insertmacro MUI_PAGE_LICENSE "resources\eula.txt" # Adds a EULA page to the installer
!insertmacro MUI_PAGE_DIRECTORY # In which folder install page.
!insertmacro MUI_PAGE_INSTFILES # Installing page.
!insertmacro MUI_PAGE_FINISH # Finished installation page.

!insertmacro MUI_UNPAGE_INSTFILES # Uninstalling page

!insertmacro MUI_LANGUAGE "English" # Set the Language of the installer

## The following two statements can be used to sign the installer and the uninstaller. The path to the binaries are provided in %1
#!uninstfinalize 'signtool --file "%1"'
#!finalize 'signtool --file "%1"'

Name "${INFO_PRODUCTNAME}"
; Output filename: ChatClaw_windows_<arch>[_full]_installer.exe
!ifdef BUNDLE_OPENCLAW
    OutFile "..\..\..\bin\${INFO_PROJECTNAME}_windows_${ARCH}_full_installer.exe"
!else
    OutFile "..\..\..\bin\${INFO_PROJECTNAME}_windows_${ARCH}_installer.exe"
!endif
InstallDir "$LOCALAPPDATA\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}" # Per-user install so auto-update works without UAC elevation.
ShowInstDetails show # This will always show the installation details.

Function .onInit
   !insertmacro wails.checkArchitecture
FunctionEnd

Section
    !insertmacro wails.setShellContext

    !insertmacro wails.webview2runtime

    SetOutPath $INSTDIR
    
    !insertmacro wails.files

    ; OpenClaw bundled CLI: must live under $INSTDIR\rt\<windows-amd64|windows-arm64> (embedded path in internal/openclaw/runtime/bundle.go).
    ; Packaged as a .zip in the installer: NSIS registers only one File entry for the zip (vs thousands of individual files
    ; if File /r were used). Extract with Windows tar.exe (bsdtar): much faster than PowerShell Expand-Archive on huge
    ; trees with many small files (e.g. node_modules). Zip layout must be flat: archive root = contents of windows-<arch>/
    ; (bin/, manifest.json, node_modules/, ...), not a nested windows-<arch>/ folder (matches Compress-Archive ...\target\*).
    !ifdef ARG_OPENCLAW_RUNTIME
        CreateDirectory "$INSTDIR\rt"
        CreateDirectory "$INSTDIR\rt\${ARG_OPENCLAW_RUNTIME_TARGET}"
        SetOutPath "$INSTDIR\rt"
        ; The zip file is compressed into the installer data section; NSIS registers only this single File line.
        File "${ARG_OPENCLAW_RUNTIME}"
        DetailPrint "Extracting OpenClaw runtime..."
        SetDetailsPrint listonly
        ExecWait 'powershell -ExecutionPolicy Bypass -WindowStyle Hidden -Command "tar -xf $INSTDIR\rt\${ARG_OPENCLAW_RUNTIME_TARGET}.zip -C $INSTDIR\rt\${ARG_OPENCLAW_RUNTIME_TARGET}"'
        Delete "$INSTDIR\rt\${ARG_OPENCLAW_RUNTIME_TARGET}.zip"
        SetDetailsPrint both
    !endif

    CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    CreateShortCut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"

    !insertmacro wails.associateFiles
    !insertmacro wails.associateCustomProtocols
    ; Register chatclaw:// URL scheme so the browser can launch the app after OAuth login
    DeleteRegKey SHELL_CONTEXT "Software\Classes\chatclaw"
    WriteRegStr SHELL_CONTEXT "Software\Classes\chatclaw" "" "URL:ChatClaw Protocol"
    WriteRegStr SHELL_CONTEXT "Software\Classes\chatclaw" "URL Protocol" ""
    WriteRegStr SHELL_CONTEXT "Software\Classes\chatclaw\DefaultIcon" "" "$INSTDIR\${PRODUCT_EXECUTABLE},0"
    WriteRegStr SHELL_CONTEXT "Software\Classes\chatclaw\shell" "" ""
    WriteRegStr SHELL_CONTEXT "Software\Classes\chatclaw\shell\open" "" ""
    WriteRegStr SHELL_CONTEXT "Software\Classes\chatclaw\shell\open\command" "" "$\"$INSTDIR\${PRODUCT_EXECUTABLE}$\" $\"%1$\""

    !insertmacro wails.writeUninstaller
SectionEnd

Section "uninstall" 
    !insertmacro wails.setShellContext

    ; Stop app and bundled Node so $INSTDIR (especially rt\) is not locked; avoids slow per-file uninstall and delete failures.
    ExecWait 'powershell.exe -ExecutionPolicy Bypass -WindowStyle Hidden -Command "taskkill /F /IM ${PRODUCT_EXECUTABLE} /T"'
    ; Stops all node.exe; may affect other Node apps during uninstall only. Narrower kill would need a bundled script.
    ExecWait 'powershell.exe -ExecutionPolicy Bypass -WindowStyle Hidden -Command "taskkill /F /IM node.exe /T"'
    Sleep 400

    RMDir /r "$AppData\${PRODUCT_EXECUTABLE}" # Remove the WebView2 DataPath

    ; Wipe rt\ in one OS call (fast; avoids NSIS RMDir walking node_modules with per-file log lines). Zip-based installs add no per-file rt Deletes.
    ; Note: brief cmd window possible; PowerShell Remove-Item line breaks NSIS ExecWait parsing (-Recurse/-Force split into extra args).
    ExecWait 'cmd /c if exist "$INSTDIR\rt" rd /s /q "$INSTDIR\rt"'

    RMDir /r $INSTDIR

    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"

    !insertmacro wails.unassociateFiles
    !insertmacro wails.unassociateCustomProtocols
    ; Remove chatclaw:// URL scheme registration
    DeleteRegKey SHELL_CONTEXT "Software\Classes\chatclaw"

    !insertmacro wails.deleteUninstaller
SectionEnd
