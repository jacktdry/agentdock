; P10 source contract only. No command-line define can enable compilation.
#error Next Windows installer disabled: runtime, payload and install/uninstall ownership gates are not accepted

#ifndef NextVersion
#error Canonical NextVersion required
#endif
#ifndef NextWindowsVersion
#error Canonical numeric NextWindowsVersion required
#endif
#ifndef NextArchitecture
#error NextArchitecture required
#endif
#ifndef NextPayloadDir
#error Verified NextPayloadDir required
#endif
#ifndef NextOutputDir
#error Fresh NextOutputDir required
#endif

#if NextArchitecture == "amd64"
#define NextAllowedArchitecture "x64compatible and not arm64"
#elif NextArchitecture == "arm64"
#define NextAllowedArchitecture "arm64"
#else
#error Unsupported NextArchitecture
#endif

[Setup]
AppId={{725CE7CB-6F4D-4D78-957E-64CF729FF413}
AppName=AgentDock Next
AppVersion={#NextVersion}
AppPublisher=AgentDock
VersionInfoVersion={#NextWindowsVersion}
VersionInfoProductVersion={#NextWindowsVersion}
VersionInfoProductName=AgentDock Next
VersionInfoDescription=AgentDock Next Setup
DefaultDirName={localappdata}\AgentDock Next
DefaultGroupName=AgentDock Next
DisableDirPage=yes
DisableProgramGroupPage=yes
UsePreviousAppDir=no
UsePreviousGroup=no
PrivilegesRequired=lowest
ArchitecturesAllowed={#NextAllowedArchitecture}
ArchitecturesInstallIn64BitMode={#NextAllowedArchitecture}
OutputDir={#NextOutputDir}
OutputBaseFilename=AgentDock-NextSetup-{#NextArchitecture}
UninstallDisplayName=AgentDock Next
UninstallDisplayIcon={app}\bin\agentdock-tray.exe
UninstallFilesDir={app}\installer
CloseApplications=no
RestartApplications=no
Compression=lzma2
SolidCompression=yes
WizardStyle=modern

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

#include "includes\next-payload.iss"

[Icons]
Name: "{userprograms}\AgentDock Next\AgentDock Next"; Filename: "{app}\bin\agentdock-tray.exe"; WorkingDir: "{app}"; AppUserModelID: "dev.dropabit.agentdock.next.controlpanel"
Name: "{userprograms}\AgentDock Next\Uninstall AgentDock Next"; Filename: "{uninstallexe}"

#include "includes\next-code.iss"
