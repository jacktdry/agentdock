; Explicit, incomplete declarative payload. No recursive Stable archive reuse.
; Same-SHA provenance and PE/assembly checks are required before this can build.
[Files]
Source: "{#NextPayloadDir}\agentdock.exe"; DestDir: "{app}\bin"; Flags: ignoreversion
Source: "{#NextPayloadDir}\agentdock-arbiter.exe"; DestDir: "{app}\bin"; Flags: ignoreversion
Source: "{#NextPayloadDir}\agentdock-tray.exe"; DestDir: "{app}\bin"; Flags: ignoreversion
Source: "{#NextPayloadDir}\candidate.json"; DestDir: "{app}\installer"; Flags: ignoreversion
