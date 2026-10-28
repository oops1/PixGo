#ifndef AppVersion
  #define AppVersion "0.0.0"
#endif
#ifndef NumericVersion
  #define NumericVersion "0.0.0.0"
#endif
#ifndef SourceDir
  #define SourceDir "..\..\dist\windows"
#endif
#ifndef LicensePage
  #define LicensePage "..\..\LICENSE"
#endif
#define AppName "PixGo"
#define AppExe "pixgo.exe"
#define ProgId "PixGo.Image"

[Setup]
AppId={{9C1D4F0A-6B7E-4E52-8C3A-2F5A91D07B48}
AppName={#AppName}
AppVersion={#AppVersion}
AppVerName={#AppName} {#AppVersion}
AppPublisher=oops1
AppPublisherURL=https://github.com/oops1/PixGo
AppSupportURL=https://github.com/oops1/PixGo/issues
AppUpdatesURL=https://github.com/oops1/PixGo/releases
VersionInfoVersion={#NumericVersion}
DefaultDirName={autopf}\{#AppName}
DefaultGroupName={#AppName}
DisableProgramGroupPage=yes
PrivilegesRequired=lowest
PrivilegesRequiredOverridesAllowed=dialog
ChangesAssociations=yes
LicenseFile={#LicensePage}
OutputDir=..\..\dist
OutputBaseFilename=pixgo-{#AppVersion}-windows-x64-setup
SetupIconFile=..\icons\pixgo.ico
UninstallDisplayIcon={app}\pixgo.ico
Compression=lzma2/ultra64
SolidCompression=yes
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
MinVersion=10.0
WizardStyle=modern
CloseApplications=yes
RestartApplications=no

[Languages]
Name: "russian"; MessagesFile: "compiler:Languages\Russian.isl"
Name: "english"; MessagesFile: "compiler:Default.isl"

[CustomMessages]
russian.AssocTask=Привязать PixGo к файлам изображений
russian.AssocGroup=Типы файлов:
russian.DesktopIconTask=Создать значок на рабочем столе
russian.OpenDefaultApps=Открыть «Приложения по умолчанию», чтобы выбрать PixGo по умолчанию
russian.ProgIdDescription=Изображение PixGo
russian.AppDescription=Быстрый и простой просмотр изображений
english.AssocTask=Associate PixGo with image files
english.AssocGroup=File types:
english.DesktopIconTask=Create a desktop icon
english.OpenDefaultApps=Open Default apps to choose PixGo as the default viewer
english.ProgIdDescription=PixGo image
english.AppDescription=A fast and simple image viewer

[Tasks]
Name: "assoc"; Description: "{cm:AssocTask}"; GroupDescription: "{cm:AssocGroup}"
Name: "desktopicon"; Description: "{cm:DesktopIconTask}"; Flags: unchecked

[Files]
Source: "{#SourceDir}\{#AppExe}"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\icons\pixgo.ico"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\LICENSE"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\NOTICE"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\third_party\*"; DestDir: "{app}\third_party"; Flags: ignoreversion recursesubdirs createallsubdirs

[Icons]
Name: "{group}\{#AppName}"; Filename: "{app}\{#AppExe}"; IconFilename: "{app}\pixgo.ico"
Name: "{autodesktop}\{#AppName}"; Filename: "{app}\{#AppExe}"; IconFilename: "{app}\pixgo.ico"; Tasks: desktopicon

[Registry]
Root: HKA; Subkey: "Software\Classes\{#ProgId}"; ValueType: string; ValueName: ""; ValueData: "{cm:ProgIdDescription}"; Flags: uninsdeletekey; Tasks: assoc
Root: HKA; Subkey: "Software\Classes\{#ProgId}\DefaultIcon"; ValueType: string; ValueName: ""; ValueData: "{app}\pixgo.ico"; Tasks: assoc
Root: HKA; Subkey: "Software\Classes\{#ProgId}\shell\open\command"; ValueType: string; ValueName: ""; ValueData: """{app}\{#AppExe}"" ""%1"""; Tasks: assoc

Root: HKA; Subkey: "Software\Classes\.apng\OpenWithProgids"; ValueType: string; ValueName: "{#ProgId}"; ValueData: ""; Flags: uninsdeletevalue; Tasks: assoc
Root: HKA; Subkey: "Software\Classes\.arw\OpenWithProgids"; ValueType: string; ValueName: "{#ProgId}"; ValueData: ""; Flags: uninsdeletevalue; Tasks: assoc
Root: HKA; Subkey: "Software\Classes\.bmp\OpenWithProgids"; ValueType: string; ValueName: "{#ProgId}"; ValueData: ""; Flags: uninsdeletevalue; Tasks: assoc
Root: HKA; Subkey: "Software\Classes\.cr2\OpenWithProgids"; ValueType: string; ValueName: "{#ProgId}"; ValueData: ""; Flags: uninsdeletevalue; Tasks: assoc
Root: HKA; Subkey: "Software\Classes\.cr3\OpenWithProgids"; ValueType: string; ValueName: "{#ProgId}"; ValueData: ""; Flags: uninsdeletevalue; Tasks: assoc
Root: HKA; Subkey: "Software\Classes\.cur\OpenWithProgids"; ValueType: string; ValueName: "{#ProgId}"; ValueData: ""; Flags: uninsdeletevalue; Tasks: assoc
Root: HKA; Subkey: "Software\Classes\.dib\OpenWithProgids"; ValueType: string; ValueName: "{#ProgId}"; ValueData: ""; Flags: uninsdeletevalue; Tasks: assoc
Root: HKA; Subkey: "Software\Classes\.dng\OpenWithProgids"; ValueType: string; ValueName: "{#ProgId}"; ValueData: ""; Flags: uninsdeletevalue; Tasks: assoc
Root: HKA; Subkey: "Software\Classes\.gif\OpenWithProgids"; ValueType: string; ValueName: "{#ProgId}"; ValueData: ""; Flags: uninsdeletevalue; Tasks: assoc
Root: HKA; Subkey: "Software\Classes\.ico\OpenWithProgids"; ValueType: string; ValueName: "{#ProgId}"; ValueData: ""; Flags: uninsdeletevalue; Tasks: assoc
Root: HKA; Subkey: "Software\Classes\.jfif\OpenWithProgids"; ValueType: string; ValueName: "{#ProgId}"; ValueData: ""; Flags: uninsdeletevalue; Tasks: assoc
Root: HKA; Subkey: "Software\Classes\.jpe\OpenWithProgids"; ValueType: string; ValueName: "{#ProgId}"; ValueData: ""; Flags: uninsdeletevalue; Tasks: assoc
Root: HKA; Subkey: "Software\Classes\.jpeg\OpenWithProgids"; ValueType: string; ValueName: "{#ProgId}"; ValueData: ""; Flags: uninsdeletevalue; Tasks: assoc
Root: HKA; Subkey: "Software\Classes\.jpg\OpenWithProgids"; ValueType: string; ValueName: "{#ProgId}"; ValueData: ""; Flags: uninsdeletevalue; Tasks: assoc
Root: HKA; Subkey: "Software\Classes\.nef\OpenWithProgids"; ValueType: string; ValueName: "{#ProgId}"; ValueData: ""; Flags: uninsdeletevalue; Tasks: assoc
Root: HKA; Subkey: "Software\Classes\.orf\OpenWithProgids"; ValueType: string; ValueName: "{#ProgId}"; ValueData: ""; Flags: uninsdeletevalue; Tasks: assoc
Root: HKA; Subkey: "Software\Classes\.pef\OpenWithProgids"; ValueType: string; ValueName: "{#ProgId}"; ValueData: ""; Flags: uninsdeletevalue; Tasks: assoc
Root: HKA; Subkey: "Software\Classes\.png\OpenWithProgids"; ValueType: string; ValueName: "{#ProgId}"; ValueData: ""; Flags: uninsdeletevalue; Tasks: assoc
Root: HKA; Subkey: "Software\Classes\.raf\OpenWithProgids"; ValueType: string; ValueName: "{#ProgId}"; ValueData: ""; Flags: uninsdeletevalue; Tasks: assoc
Root: HKA; Subkey: "Software\Classes\.rw2\OpenWithProgids"; ValueType: string; ValueName: "{#ProgId}"; ValueData: ""; Flags: uninsdeletevalue; Tasks: assoc
Root: HKA; Subkey: "Software\Classes\.svg\OpenWithProgids"; ValueType: string; ValueName: "{#ProgId}"; ValueData: ""; Flags: uninsdeletevalue; Tasks: assoc
Root: HKA; Subkey: "Software\Classes\.svgz\OpenWithProgids"; ValueType: string; ValueName: "{#ProgId}"; ValueData: ""; Flags: uninsdeletevalue; Tasks: assoc
Root: HKA; Subkey: "Software\Classes\.tif\OpenWithProgids"; ValueType: string; ValueName: "{#ProgId}"; ValueData: ""; Flags: uninsdeletevalue; Tasks: assoc
Root: HKA; Subkey: "Software\Classes\.tiff\OpenWithProgids"; ValueType: string; ValueName: "{#ProgId}"; ValueData: ""; Flags: uninsdeletevalue; Tasks: assoc
Root: HKA; Subkey: "Software\Classes\.webp\OpenWithProgids"; ValueType: string; ValueName: "{#ProgId}"; ValueData: ""; Flags: uninsdeletevalue; Tasks: assoc
Root: HKA; Subkey: "Software\Classes\.avif\OpenWithProgids"; ValueType: string; ValueName: "{#ProgId}"; ValueData: ""; Flags: uninsdeletevalue; Tasks: assoc
Root: HKA; Subkey: "Software\Classes\.heic\OpenWithProgids"; ValueType: string; ValueName: "{#ProgId}"; ValueData: ""; Flags: uninsdeletevalue; Tasks: assoc
Root: HKA; Subkey: "Software\Classes\.heif\OpenWithProgids"; ValueType: string; ValueName: "{#ProgId}"; ValueData: ""; Flags: uninsdeletevalue; Tasks: assoc
Root: HKA; Subkey: "Software\Classes\.hif\OpenWithProgids"; ValueType: string; ValueName: "{#ProgId}"; ValueData: ""; Flags: uninsdeletevalue; Tasks: assoc
Root: HKA; Subkey: "Software\Classes\.jxl\OpenWithProgids"; ValueType: string; ValueName: "{#ProgId}"; ValueData: ""; Flags: uninsdeletevalue; Tasks: assoc
Root: HKA; Subkey: "Software\Classes\.stl\OpenWithProgids"; ValueType: string; ValueName: "{#ProgId}"; ValueData: ""; Flags: uninsdeletevalue; Tasks: assoc
Root: HKA; Subkey: "Software\Classes\.step\OpenWithProgids"; ValueType: string; ValueName: "{#ProgId}"; ValueData: ""; Flags: uninsdeletevalue; Tasks: assoc
Root: HKA; Subkey: "Software\Classes\.stp\OpenWithProgids"; ValueType: string; ValueName: "{#ProgId}"; ValueData: ""; Flags: uninsdeletevalue; Tasks: assoc

Root: HKA; Subkey: "Software\Classes\Applications\{#AppExe}"; ValueType: string; ValueName: "FriendlyAppName"; ValueData: "{#AppName}"; Flags: uninsdeletekey; Tasks: assoc
Root: HKA; Subkey: "Software\Classes\Applications\{#AppExe}\shell\open\command"; ValueType: string; ValueName: ""; ValueData: """{app}\{#AppExe}"" ""%1"""; Tasks: assoc

Root: HKA; Subkey: "Software\PixGo"; Flags: uninsdeletekeyifempty; Tasks: assoc
Root: HKA; Subkey: "Software\PixGo\Capabilities"; ValueType: string; ValueName: "ApplicationName"; ValueData: "{#AppName}"; Flags: uninsdeletekey; Tasks: assoc
Root: HKA; Subkey: "Software\PixGo\Capabilities"; ValueType: string; ValueName: "ApplicationDescription"; ValueData: "{cm:AppDescription}"; Tasks: assoc
Root: HKA; Subkey: "Software\PixGo\Capabilities"; ValueType: string; ValueName: "ApplicationIcon"; ValueData: "{app}\pixgo.ico,0"; Tasks: assoc
Root: HKA; Subkey: "Software\PixGo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".apng"; ValueData: "{#ProgId}"; Tasks: assoc
Root: HKA; Subkey: "Software\PixGo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".arw"; ValueData: "{#ProgId}"; Tasks: assoc
Root: HKA; Subkey: "Software\PixGo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".bmp"; ValueData: "{#ProgId}"; Tasks: assoc
Root: HKA; Subkey: "Software\PixGo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".cr2"; ValueData: "{#ProgId}"; Tasks: assoc
Root: HKA; Subkey: "Software\PixGo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".cr3"; ValueData: "{#ProgId}"; Tasks: assoc
Root: HKA; Subkey: "Software\PixGo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".cur"; ValueData: "{#ProgId}"; Tasks: assoc
Root: HKA; Subkey: "Software\PixGo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".dib"; ValueData: "{#ProgId}"; Tasks: assoc
Root: HKA; Subkey: "Software\PixGo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".dng"; ValueData: "{#ProgId}"; Tasks: assoc
Root: HKA; Subkey: "Software\PixGo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".gif"; ValueData: "{#ProgId}"; Tasks: assoc
Root: HKA; Subkey: "Software\PixGo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".ico"; ValueData: "{#ProgId}"; Tasks: assoc
Root: HKA; Subkey: "Software\PixGo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".jfif"; ValueData: "{#ProgId}"; Tasks: assoc
Root: HKA; Subkey: "Software\PixGo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".jpe"; ValueData: "{#ProgId}"; Tasks: assoc
Root: HKA; Subkey: "Software\PixGo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".jpeg"; ValueData: "{#ProgId}"; Tasks: assoc
Root: HKA; Subkey: "Software\PixGo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".jpg"; ValueData: "{#ProgId}"; Tasks: assoc
Root: HKA; Subkey: "Software\PixGo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".nef"; ValueData: "{#ProgId}"; Tasks: assoc
Root: HKA; Subkey: "Software\PixGo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".orf"; ValueData: "{#ProgId}"; Tasks: assoc
Root: HKA; Subkey: "Software\PixGo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".pef"; ValueData: "{#ProgId}"; Tasks: assoc
Root: HKA; Subkey: "Software\PixGo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".png"; ValueData: "{#ProgId}"; Tasks: assoc
Root: HKA; Subkey: "Software\PixGo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".raf"; ValueData: "{#ProgId}"; Tasks: assoc
Root: HKA; Subkey: "Software\PixGo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".rw2"; ValueData: "{#ProgId}"; Tasks: assoc
Root: HKA; Subkey: "Software\PixGo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".svg"; ValueData: "{#ProgId}"; Tasks: assoc
Root: HKA; Subkey: "Software\PixGo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".svgz"; ValueData: "{#ProgId}"; Tasks: assoc
Root: HKA; Subkey: "Software\PixGo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".tif"; ValueData: "{#ProgId}"; Tasks: assoc
Root: HKA; Subkey: "Software\PixGo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".tiff"; ValueData: "{#ProgId}"; Tasks: assoc
Root: HKA; Subkey: "Software\PixGo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".webp"; ValueData: "{#ProgId}"; Tasks: assoc
Root: HKA; Subkey: "Software\PixGo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".avif"; ValueData: "{#ProgId}"; Tasks: assoc
Root: HKA; Subkey: "Software\PixGo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".heic"; ValueData: "{#ProgId}"; Tasks: assoc
Root: HKA; Subkey: "Software\PixGo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".heif"; ValueData: "{#ProgId}"; Tasks: assoc
Root: HKA; Subkey: "Software\PixGo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".hif"; ValueData: "{#ProgId}"; Tasks: assoc
Root: HKA; Subkey: "Software\PixGo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".jxl"; ValueData: "{#ProgId}"; Tasks: assoc
Root: HKA; Subkey: "Software\PixGo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".stl"; ValueData: "{#ProgId}"; Tasks: assoc
Root: HKA; Subkey: "Software\PixGo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".step"; ValueData: "{#ProgId}"; Tasks: assoc
Root: HKA; Subkey: "Software\PixGo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".stp"; ValueData: "{#ProgId}"; Tasks: assoc
Root: HKA; Subkey: "Software\RegisteredApplications"; ValueType: string; ValueName: "{#AppName}"; ValueData: "Software\PixGo\Capabilities"; Flags: uninsdeletevalue; Tasks: assoc

[Run]
Filename: "ms-settings:defaultapps?registeredAppUser={#AppName}"; Description: "{cm:OpenDefaultApps}"; Flags: shellexec postinstall skipifsilent unchecked; Tasks: assoc
Filename: "{app}\{#AppExe}"; Description: "{cm:LaunchProgram,{#AppName}}"; Flags: nowait postinstall skipifsilent
