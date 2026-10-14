[Code]
function IsNextRoot(Path: String): Boolean;
begin
  Result := CompareText(RemoveBackslash(ExpandFileName(Path)),
    RemoveBackslash(ExpandFileName(ExpandConstant('{localappdata}\AgentDock Next')))) = 0;
end;

function PrepareToInstall(var NeedsRestart: Boolean): String;
begin
  Result := '';
  { DisableDirPage alone does not prevent /DIR or a saved INF override. }
  if not IsNextRoot(WizardDirValue) then
    Result := 'AgentDock Next requires its independent LocalAppData\AgentDock Next directory.';
end;

function InitializeUninstall(): Boolean;
begin
  Result := IsNextRoot(ExpandConstant('{app}'));
  if not Result then
    Log('AgentDock Next uninstall rejected: unexpected installation directory.');
end;
