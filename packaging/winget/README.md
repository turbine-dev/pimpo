# winget

Pimpo is published to winget as `TurbineDev.Pimpo`, from the NSIS setup of each final release (`Pimpo_X.Y.Z_windows_amd64_setup.exe`).

**The first version** is submitted by hand, since winget reviews every new package:

```powershell
winget install wingetcreate
wingetcreate new https://github.com/turbine-dev/pimpo/releases/download/vX.Y.Z/Pimpo_X.Y.Z_windows_amd64_setup.exe
```

Answer the questions with: publisher `Turbine Dev`, package identifier `TurbineDev.Pimpo`, name `Pimpo`, license `Apache-2.0`, short description "Personal AI agent that learns a task once, then does it on its own", homepage `https://github.com/turbine-dev/pimpo`. `wingetcreate` opens the pull request to `microsoft/winget-pkgs`.

**Later versions** are submitted by the release workflow (`desktop.yml`, job `manifest`) when the repository has a `WINGET_TOKEN` secret: a classic GitHub token with `public_repo` from an account that has a fork of `microsoft/winget-pkgs`.
